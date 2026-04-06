package handler

import (
	"github.com/golangsnmp/gomib/mib"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) textDocumentDocumentSymbol(ctx *glsp.Context, params *protocol.DocumentSymbolParams) (any, error) {
	uri := params.TextDocument.URI

	s.mu.Lock()
	doc := s.documents[uri]
	m := s.mib
	s.mu.Unlock()

	if doc == nil || m == nil {
		return nil, nil
	}

	var modules []protocol.DocumentSymbol

	for _, modName := range doc.modules {
		mod := m.Module(modName)
		if mod == nil {
			continue
		}

		var children []protocol.DocumentSymbol
		for def := range mod.Definitions() {
			kind := symbolKindFromMib(def.Kind())
			detail := def.Kind().MacroName()
			if sym, ok := defSymbol(mod, def.Name(), def.Span(), detail, kind); ok {
				children = append(children, sym)
			}
		}

		if len(children) == 0 {
			continue
		}

		modRange := enclosingRange(children)
		modSym := protocol.DocumentSymbol{
			Name:           modName,
			Kind:           protocol.SymbolKindModule,
			Range:          modRange,
			SelectionRange: modRange,
			Children:       children,
		}
		modules = append(modules, modSym)
	}

	if len(modules) == 0 {
		return nil, nil
	}
	return modules, nil
}

// enclosingRange computes the smallest range that contains all the given symbols.
// The caller must ensure symbols is non-empty.
func enclosingRange(symbols []protocol.DocumentSymbol) protocol.Range {
	r := symbols[0].Range
	for _, s := range symbols[1:] {
		if s.Range.Start.Line < r.Start.Line || (s.Range.Start.Line == r.Start.Line && s.Range.Start.Character < r.Start.Character) {
			r.Start = s.Range.Start
		}
		if s.Range.End.Line > r.End.Line || (s.Range.End.Line == r.End.Line && s.Range.End.Character > r.End.Character) {
			r.End = s.Range.End
		}
	}
	return r
}

// defSymbol builds a DocumentSymbol from a definition's name, span, and kind.
// Returns false if the span has no valid source location (synthetic definitions).
func defSymbol(mod *mib.Module, name string, span mib.Span, detail string, kind protocol.SymbolKind) (protocol.DocumentSymbol, bool) {
	if name == "" {
		return protocol.DocumentSymbol{}, false
	}
	r, ok := spanToRange(mod, span)
	if !ok {
		return protocol.DocumentSymbol{}, false
	}
	sym := protocol.DocumentSymbol{
		Name:           name,
		Kind:           kind,
		Range:          r,
		SelectionRange: r,
	}
	if detail != "" {
		sym.Detail = &detail
	}
	return sym, true
}

// symbolKindFromMib maps a mib.SymbolKind to an LSP SymbolKind.
// This is the canonical mapping for the LSP; completion kinds and
// semantic token types derive from it.
func symbolKindFromMib(k mib.SymbolKind) protocol.SymbolKind {
	switch k {
	case mib.SymbolKindObject:
		return protocol.SymbolKindVariable
	case mib.SymbolKindTextualConvention, mib.SymbolKindType:
		return protocol.SymbolKindClass
	case mib.SymbolKindNotification:
		return protocol.SymbolKindEvent
	case mib.SymbolKindGroup, mib.SymbolKindNotificationGroup:
		return protocol.SymbolKindNamespace
	case mib.SymbolKindCompliance, mib.SymbolKindCapability:
		return protocol.SymbolKindInterface
	default:
		return protocol.SymbolKindConstant
	}
}

// spanToRange converts a mib.Span to an LSP Range using the module's line table.
// Returns false if the span has no valid source location.
func spanToRange(mod *mib.Module, span mib.Span) (protocol.Range, bool) {
	startLine, startCol := mod.LineCol(span.Start)
	endLine, endCol := mod.LineCol(span.End)
	if startLine == 0 {
		return protocol.Range{}, false
	}
	return protocol.Range{
		Start: protocol.Position{
			Line:      protocol.UInteger(startLine - 1),
			Character: protocol.UInteger(startCol - 1),
		},
		End: protocol.Position{
			Line:      protocol.UInteger(endLine - 1),
			Character: protocol.UInteger(endCol - 1),
		},
	}, true
}
