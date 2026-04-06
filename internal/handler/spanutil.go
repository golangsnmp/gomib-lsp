package handler

import (
	"github.com/golangsnmp/gomib/mib"
	"github.com/golangsnmp/gomib/syntax"
)

// isIdentChar returns true for characters valid in SMI identifiers.
// Used for text-based word boundary detection (completion prefix, text search).
func isIdentChar(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '-' || b == '_'
}

// lspOffset converts a 0-based LSP position to a byte offset using the
// document's line table. Returns the offset and true, or 0 and false if
// the position is out of range.
func lspOffset(doc *document, line, col int) (syntax.ByteOffset, bool) {
	return doc.lineTable.Offset(line+1, col+1) // LineTable is 1-based
}

// tokenText returns the source text of the CST token at the given byte offset,
// or "" if the offset doesn't fall on a token (e.g. whitespace, comment).
func tokenText(doc *document, offset syntax.ByteOffset) string {
	tok, ok := syntax.TokenAt(doc.cst, offset)
	if !ok {
		return ""
	}
	return doc.content[tok.Span.Start:tok.Span.End]
}

// moduleForPosition returns the module in the document for span lookups.
// For single-module files (the common case), returns that module directly.
func moduleForPosition(doc *document, m *mib.Mib) *mib.Module {
	if m == nil || len(doc.modules) == 0 {
		return nil
	}
	if len(doc.modules) == 1 {
		return m.Module(doc.modules[0])
	}
	// Multi-module file: return the first available module.
	for _, name := range doc.modules {
		if mod := m.Module(name); mod != nil {
			return mod
		}
	}
	return nil
}
