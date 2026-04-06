package handler

import (
	"github.com/golangsnmp/gomib/mib"
	"github.com/golangsnmp/gomib/syntax"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// Semantic token type indices.
const (
	semTypeNamespace  = 0
	semTypeType       = 1
	semTypeClass      = 2
	semTypeInterface  = 3
	semTypeVariable   = 4
	semTypeEnumMember = 5
	semTypeEvent      = 6
	semTypeMacro      = 7
	semTypeKeyword    = 8
	semTypeComment    = 9
	semTypeString     = 10
	semTypeNumber     = 11
	semTypeOperator   = 12
)

// Semantic token modifier bit indices.
const (
	semModDefinition     = 0
	semModDeprecated     = 1
	semModReadonly       = 2
	semModDefaultLibrary = 3
)

var semanticTokenTypes = []string{
	"namespace",
	"type",
	"class",
	"interface",
	"variable",
	"enumMember",
	"event",
	"macro",
	"keyword",
	"comment",
	"string",
	"number",
	"operator",
}

var semanticTokenModifiers = []string{
	"definition",
	"deprecated",
	"readonly",
	"defaultLibrary",
}

func semanticTokensLegend() protocol.SemanticTokensLegend {
	return protocol.SemanticTokensLegend{
		TokenTypes:     semanticTokenTypes,
		TokenModifiers: semanticTokenModifiers,
	}
}

func (s *Server) textDocumentSemanticTokensFull(ctx *glsp.Context, params *protocol.SemanticTokensParams) (*protocol.SemanticTokens, error) {
	uri := params.TextDocument.URI

	s.mu.Lock()
	doc := s.documents[uri]
	m := s.mib
	s.mu.Unlock()

	if doc == nil {
		return nil, nil
	}

	source := []byte(doc.content)
	tokens := syntax.Tokenize(source)
	data := encodeSemanticTokens(source, tokens, doc.lineTable, m)
	if len(data) == 0 {
		return nil, nil
	}
	return &protocol.SemanticTokens{Data: data}, nil
}

// encodeSemanticTokens classifies each lexer token and produces the
// delta-encoded data array for the LSP semantic tokens response.
func encodeSemanticTokens(source []byte, tokens []syntax.Token, lt syntax.LineTable, m *mib.Mib) []protocol.UInteger {
	data := make([]protocol.UInteger, 0, len(tokens)*5)

	var prevLine, prevCol protocol.UInteger

	for _, tok := range tokens {
		if tok.Kind == syntax.TokEOF {
			break
		}

		tokenType, modifiers, ok := classifyToken(source, tok, m)
		if !ok {
			continue
		}

		l, c := lt.LineCol(tok.Span.Start)
		line := protocol.UInteger(l - 1) // LineTable is 1-based, LSP is 0-based
		col := protocol.UInteger(c - 1)
		length := protocol.UInteger(tok.Span.End - tok.Span.Start)

		deltaLine := line - prevLine
		deltaCol := col
		if deltaLine == 0 {
			deltaCol = col - prevCol
		}

		data = append(data, deltaLine, deltaCol, length, tokenType, modifiers)
		prevLine = line
		prevCol = col
	}

	return data
}

// classifyToken maps a lexer token to a semantic token type and modifiers.
// Returns ok=false if the token should be skipped (no semantic token emitted).
func classifyToken(source []byte, tok syntax.Token, m *mib.Mib) (tokenType, modifiers protocol.UInteger, ok bool) {
	k := tok.Kind

	switch {
	case k == syntax.TokComment:
		return semTypeComment, 0, true

	case k == syntax.TokQuotedString || k == syntax.TokHexString || k == syntax.TokBinString:
		return semTypeString, 0, true

	case k == syntax.TokNumber || k == syntax.TokNegativeNumber:
		return semTypeNumber, 0, true

	case k.IsMacroKeyword():
		return semTypeMacro, 0, true

	case k.IsClauseKeyword():
		return semTypeKeyword, 0, true

	case k.IsTypeKeyword():
		return semTypeType, 0, true

	case k.IsTagKeyword():
		return semTypeKeyword, 0, true

	case k.IsStatusAccessKeyword():
		return semTypeEnumMember, 0, true

	case k.IsStructuralKeyword():
		return semTypeKeyword, 0, true

	case k == syntax.TokColonColonEqual:
		return semTypeOperator, 0, true

	case k == syntax.TokUppercaseIdent || k == syntax.TokLowercaseIdent:
		return classifyIdentifier(source, tok, m)

	default:
		return 0, 0, false
	}
}

// classifyIdentifier uses the resolved model to classify identifier tokens.
func classifyIdentifier(source []byte, tok syntax.Token, m *mib.Mib) (tokenType, modifiers protocol.UInteger, ok bool) {
	if m == nil {
		return 0, 0, false
	}

	text := string(source[tok.Span.Start:tok.Span.End])

	if m.Module(text) != nil {
		return semTypeNamespace, 0, true
	}

	sym := m.Symbol(text)
	if sym.IsZero() {
		return 0, 0, false
	}

	tt := symbolTokenType(sym)
	mods := symbolModifiers(sym, tok)
	return tt, mods, true
}

// symbolTokenType maps a Symbol to a semantic token type index,
// derived from symbolKindFromMib.
func symbolTokenType(sym mib.Symbol) protocol.UInteger {
	return symbolKindToTokenType(symbolKindFromMib(sym.Kind()))
}

func symbolKindToTokenType(k protocol.SymbolKind) protocol.UInteger {
	switch k {
	case protocol.SymbolKindVariable:
		return semTypeVariable
	case protocol.SymbolKindClass:
		return semTypeClass
	case protocol.SymbolKindEvent:
		return semTypeEvent
	case protocol.SymbolKindNamespace:
		return semTypeNamespace
	case protocol.SymbolKindInterface:
		return semTypeInterface
	default:
		return semTypeVariable
	}
}

// symbolModifiers computes the modifier bitmask for a symbol at a token position.
func symbolModifiers(sym mib.Symbol, tok syntax.Token) protocol.UInteger {
	var mods protocol.UInteger

	// Definition site: token start matches symbol span start.
	if sym.Span().Start == tok.Span.Start {
		mods |= 1 << semModDefinition
	}

	// Deprecated/obsolete.
	st := sym.Status()
	if st == mib.StatusDeprecated || st == mib.StatusObsolete {
		mods |= 1 << semModDeprecated
	}

	// Read-only access.
	if obj := sym.Object(); obj != nil && obj.Access() == mib.AccessReadOnly {
		mods |= 1 << semModReadonly
	}

	// Base module symbol.
	if mod := sym.Module(); mod != nil && mod.IsBase() {
		mods |= 1 << semModDefaultLibrary
	}

	return mods
}
