package handler

import (
	"github.com/golangsnmp/gomib/mib"
	"github.com/golangsnmp/gomib/syntax"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) textDocumentDefinition(ctx *glsp.Context, params *protocol.DefinitionParams) (any, error) {
	uri := params.TextDocument.URI

	s.mu.Lock()
	doc := s.documents[uri]
	m := s.mib
	s.mu.Unlock()

	if doc == nil || m == nil {
		return nil, nil
	}

	offset, ok := lspOffset(doc, int(params.Position.Line), int(params.Position.Character))
	if !ok {
		return nil, nil
	}

	if doc.cst == nil {
		return nil, nil
	}

	// Try CST-based lookup first for precise navigation.
	if loc := s.cstDefinition(doc, m, offset); loc != nil {
		return *loc, nil
	}

	// Fall back to word-based lookup.
	word := tokenText(doc, offset)
	if word == "" {
		return nil, nil
	}

	return definitionLocation(m, word)
}

// cstDefinition uses the CST cursor context to resolve go-to-definition.
func (s *Server) cstDefinition(doc *document, m *mib.Mib, offset syntax.ByteOffset) *protocol.Location {
	cctx := syntax.CursorContextAt(doc.cst, offset)

	// Navigate to the definition of an import symbol or OID reference.
	if cctx.ImportGroup != nil || cctx.OidValue != nil {
		word := tokenText(doc, offset)
		if word == "" {
			return nil
		}
		loc, _ := definitionLocation(m, word)
		if loc == nil {
			return nil
		}
		l := loc.(protocol.Location)
		return &l
	}

	// Within a definition, navigate to type definitions in SYNTAX clauses.
	if cctx.Clause == syntax.ClauseSyntax || cctx.Clause == syntax.ClauseWriteSyntax {
		word := tokenText(doc, offset)
		if word == "" {
			return nil
		}
		t := m.Type(word)
		if t == nil {
			return nil
		}
		tMod := t.Module()
		if tMod == nil || tMod.SourcePath() == "" {
			return nil
		}
		r, ok := spanToRange(tMod, t.Span())
		if !ok {
			return nil
		}
		return &protocol.Location{URI: pathToURI(tMod.SourcePath()), Range: r}
	}

	return nil
}

// definitionLocation returns the Location for a named MIB symbol, or nil.
func definitionLocation(m *mib.Mib, name string) (any, error) {
	mod, span := symbolDefinition(m, name)
	if mod == nil || mod.SourcePath() == "" {
		return nil, nil
	}

	r, ok := spanToRange(mod, span)
	if !ok {
		return nil, nil
	}

	return protocol.Location{
		URI:   pathToURI(mod.SourcePath()),
		Range: r,
	}, nil
}

// symbolDefinition returns the module and span for the definition of a MIB symbol.
func symbolDefinition(m *mib.Mib, name string) (*mib.Module, mib.Span) {
	sym := m.Symbol(name)
	if sym.IsZero() {
		return nil, mib.Span{}
	}
	return sym.Module(), sym.Span()
}
