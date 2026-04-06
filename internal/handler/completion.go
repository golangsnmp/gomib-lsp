package handler

import (
	"strings"

	"github.com/golangsnmp/gomib/mib"
	"github.com/golangsnmp/gomib/syntax"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) textDocumentCompletion(ctx *glsp.Context, params *protocol.CompletionParams) (any, error) {
	uri := params.TextDocument.URI

	s.mu.Lock()
	doc := s.documents[uri]
	m := s.mib
	s.mu.Unlock()

	if doc == nil || doc.cst == nil || m == nil {
		return nil, nil
	}

	offset, ok := lspOffset(doc, int(params.Position.Line), int(params.Position.Character))
	if !ok {
		return nil, nil
	}

	cctx := syntax.CursorContextAt(doc.cst, offset)

	if cctx.InComment || cctx.InString {
		return nil, nil
	}

	text := doc.content[:offset]
	prefix := partialWordBeforeCursor(text)
	mod := moduleForPosition(doc, m)

	var items []protocol.CompletionItem

	switch {
	case cctx.Imports != nil:
		if isAfterFROM(cctx, offset) {
			items = importModuleCompletions(m)
		} else {
			items = importSymbolCompletions(m)
		}
	case cctx.OidValue != nil:
		items = oidParentCompletions(mod)
	case cctx.Clause == syntax.ClauseStatus:
		items = statusCompletions()
	case cctx.Clause == syntax.ClauseAccess:
		items = accessCompletions()
	case cctx.Clause == syntax.ClauseSyntax || cctx.Clause == syntax.ClauseWriteSyntax:
		items = syntaxCompletions(mod)
	default:
		items = generalCompletions(mod, m)
	}

	if prefix != "" {
		items = prefixFilter(items, prefix)
	}

	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

// isAfterFROM returns true when the cursor is after a FROM keyword within
// an import group (i.e., expecting a module name).
func isAfterFROM(cctx syntax.CursorContext, offset syntax.ByteOffset) bool {
	if cctx.ImportGroup == nil {
		return false
	}
	from := cctx.ImportGroup.From
	return !from.IsZero() && offset >= from.Span.End
}

// partialWordBeforeCursor extracts the partial identifier immediately before
// the cursor position.
func partialWordBeforeCursor(text string) string {
	i := len(text) - 1
	for i >= 0 && isIdentChar(text[i]) {
		i--
	}
	return text[i+1:]
}

// statusCompletions returns completion items for STATUS clause values.
func statusCompletions() []protocol.CompletionItem {
	return enumCompletions(mib.StatusNames())
}

// accessCompletions returns completion items for MAX-ACCESS clause values.
func accessCompletions() []protocol.CompletionItem {
	all := mib.AccessNames()
	names := make([]string, 0, len(all))
	for _, n := range all {
		if n != "not-implemented" {
			names = append(names, n)
		}
	}
	return enumCompletions(names)
}

// enumCompletions builds completion items for a fixed set of enum-like values.
func enumCompletions(values []string) []protocol.CompletionItem {
	items := make([]protocol.CompletionItem, len(values))
	for i, v := range values {
		kind := protocol.CompletionItemKindEnumMember
		items[i] = protocol.CompletionItem{
			Label:    v,
			Kind:     &kind,
			SortText: ptrTo("0" + v),
		}
	}
	return items
}

// syntaxCompletions returns type names available in the current module scope,
// plus static base type names.
func syntaxCompletions(mod *mib.Module) []protocol.CompletionItem {
	baseTypes := mib.SyntaxBaseTypeNames()

	kind := protocol.CompletionItemKindClass
	items := make([]protocol.CompletionItem, 0, len(baseTypes))
	for _, name := range baseTypes {
		items = append(items, protocol.CompletionItem{
			Label:    name,
			Kind:     &kind,
			SortText: ptrTo("0" + strings.ToLower(name)),
			Detail:   ptrTo("base type"),
		})
	}

	if mod == nil {
		return items
	}

	seen := make(map[string]struct{})
	for _, name := range baseTypes {
		seen[name] = struct{}{}
	}

	for sym := range mod.AvailableSymbols() {
		if sym.Type() == nil {
			continue
		}
		name := sym.Name()
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		items = append(items, completionItemFromSymbol(sym, mod))
	}

	return items
}

// oidParentCompletions returns node names from the current module's scope.
func oidParentCompletions(mod *mib.Module) []protocol.CompletionItem {
	if mod == nil {
		return nil
	}

	var items []protocol.CompletionItem
	for sym := range mod.AvailableSymbols() {
		if sym.Node() == nil {
			continue
		}
		items = append(items, completionItemFromSymbol(sym, mod))
	}
	return items
}

// importModuleCompletions returns loaded module names, excluding base modules.
func importModuleCompletions(m *mib.Mib) []protocol.CompletionItem {
	kind := protocol.CompletionItemKindModule
	var items []protocol.CompletionItem
	for _, mod := range m.Modules() {
		if mod.IsBase() {
			continue
		}
		items = append(items, protocol.CompletionItem{
			Label:    mod.Name(),
			Kind:     &kind,
			SortText: ptrTo("0" + strings.ToLower(mod.Name())),
		})
	}
	return items
}

// importSymbolCompletions returns exported symbols from all non-base modules.
func importSymbolCompletions(m *mib.Mib) []protocol.CompletionItem {
	seen := make(map[string]struct{})
	var items []protocol.CompletionItem

	for _, mod := range m.Modules() {
		if mod.IsBase() {
			continue
		}
		for sym := range mod.ExportedSymbols() {
			name := sym.Name()
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}

			kind := completionKindFromSymbol(sym)
			item := protocol.CompletionItem{
				Label:    name,
				Kind:     &kind,
				SortText: ptrTo("0" + strings.ToLower(name)),
				Detail:   ptrTo(mod.Name()),
			}
			addDeprecatedTag(&item, sym)
			items = append(items, item)
		}
	}
	return items
}

// generalCompletions returns all symbols available in the current module scope.
func generalCompletions(mod *mib.Module, m *mib.Mib) []protocol.CompletionItem {
	if mod != nil {
		return availableSymbolCompletions(mod)
	}
	if m == nil {
		return nil
	}
	// No module context, fall back to all symbols.
	var items []protocol.CompletionItem
	for sym := range m.AllSymbols() {
		items = append(items, completionItemFromSymbol(sym, nil))
	}
	return items
}

// availableSymbolCompletions builds completion items from a module's available symbols.
func availableSymbolCompletions(mod *mib.Module) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for sym := range mod.AvailableSymbols() {
		items = append(items, completionItemFromSymbol(sym, mod))
	}
	return items
}

// completionItemFromSymbol builds a CompletionItem from a Symbol.
func completionItemFromSymbol(sym mib.Symbol, currentMod *mib.Module) protocol.CompletionItem {
	name := sym.Name()
	kind := completionKindFromSymbol(sym)

	// Local definitions sort before imports.
	sortPrefix := "1"
	if currentMod != nil && sym.Module() == currentMod {
		sortPrefix = "0"
	}

	item := protocol.CompletionItem{
		Label:    name,
		Kind:     &kind,
		SortText: ptrTo(sortPrefix + strings.ToLower(name)),
	}

	// Add detail string.
	if oid := sym.OID(); len(oid) > 0 {
		item.Detail = ptrTo(oid.String())
	} else if detail := sym.Kind().MacroName(); detail != "" {
		item.Detail = ptrTo(detail)
	}

	addDeprecatedTag(&item, sym)
	return item
}

// completionKindFromSymbol maps a Symbol to a CompletionItemKind,
// derived from symbolKindFromMib.
func completionKindFromSymbol(sym mib.Symbol) protocol.CompletionItemKind {
	return symbolKindToCompletion(symbolKindFromMib(sym.Kind()))
}

func symbolKindToCompletion(k protocol.SymbolKind) protocol.CompletionItemKind {
	switch k {
	case protocol.SymbolKindVariable:
		return protocol.CompletionItemKindVariable
	case protocol.SymbolKindClass:
		return protocol.CompletionItemKindClass
	case protocol.SymbolKindEvent:
		return protocol.CompletionItemKindEvent
	case protocol.SymbolKindNamespace:
		return protocol.CompletionItemKindModule
	case protocol.SymbolKindInterface:
		return protocol.CompletionItemKindInterface
	default:
		return protocol.CompletionItemKindConstant
	}
}

// addDeprecatedTag adds the deprecated completion item tag if the symbol has
// StatusDeprecated or StatusObsolete.
func addDeprecatedTag(item *protocol.CompletionItem, sym mib.Symbol) {
	status := sym.Status()
	if status == mib.StatusDeprecated || status == mib.StatusObsolete {
		item.Tags = []protocol.CompletionItemTag{protocol.CompletionItemTagDeprecated}
	}
}

// prefixFilter filters completion items to those whose label starts with prefix
// (case-insensitive).
func prefixFilter(items []protocol.CompletionItem, prefix string) []protocol.CompletionItem {
	lower := strings.ToLower(prefix)
	filtered := make([]protocol.CompletionItem, 0, len(items))
	for _, item := range items {
		if strings.HasPrefix(strings.ToLower(item.Label), lower) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
