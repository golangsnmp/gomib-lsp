package handler

import (
	"os"
	"strings"

	"github.com/golangsnmp/gomib/mib"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) textDocumentReferences(ctx *glsp.Context, params *protocol.ReferenceParams) ([]protocol.Location, error) {
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

	word := tokenText(doc, offset)
	if word == "" {
		return nil, nil
	}

	// Verify the word is a known MIB symbol.
	defMod, _ := symbolDefinition(m, word)
	if defMod == nil {
		return nil, nil
	}

	// Text-based references in the current document.
	locs := findWordOccurrences(doc.content, word, uri)

	// Text-based cross-file references.
	filePath := uriToPath(uri)
	locs = append(locs, crossFileReferences(m, word, filePath)...)

	// Merge semantic span-based references.
	locs = mergeLocations(locs, semanticReferences(m, word))

	if !params.Context.IncludeDeclaration {
		locs = excludeDefinition(locs, m, word)
	}

	if len(locs) == 0 {
		return nil, nil
	}
	return locs, nil
}

// findWordOccurrences scans content for whole-word occurrences of word,
// returning an LSP Location for each match.
func findWordOccurrences(content, word string, uri protocol.DocumentUri) []protocol.Location {
	var locs []protocol.Location
	lineNum := 0
	rest := content

	for rest != "" {
		lineEnd := strings.IndexByte(rest, '\n')
		var line string
		if lineEnd < 0 {
			line = rest
			rest = ""
		} else {
			line = rest[:lineEnd]
			rest = rest[lineEnd+1:]
		}

		col := 0
		for col <= len(line)-len(word) {
			idx := strings.Index(line[col:], word)
			if idx < 0 {
				break
			}
			matchCol := col + idx
			endCol := matchCol + len(word)

			before := matchCol > 0 && isIdentChar(line[matchCol-1])
			after := endCol < len(line) && isIdentChar(line[endCol])

			if !before && !after {
				locs = append(locs, protocol.Location{
					URI: uri,
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      protocol.UInteger(lineNum),
							Character: protocol.UInteger(matchCol),
						},
						End: protocol.Position{
							Line:      protocol.UInteger(lineNum),
							Character: protocol.UInteger(endCol),
						},
					},
				})
			}

			col = endCol
		}

		lineNum++
	}

	return locs
}

// crossFileReferences scans source files for all loaded modules (except the
// current file) for occurrences of word.
func crossFileReferences(m *mib.Mib, word, currentPath string) []protocol.Location {
	seen := map[string]struct{}{}
	if currentPath != "" {
		seen[currentPath] = struct{}{}
	}

	var locs []protocol.Location
	for _, mod := range m.Modules() {
		sp := mod.SourcePath()
		if sp == "" {
			continue
		}
		if _, ok := seen[sp]; ok {
			continue
		}
		seen[sp] = struct{}{}

		data, err := os.ReadFile(sp)
		if err != nil {
			continue
		}
		locs = append(locs, findWordOccurrences(string(data), word, pathToURI(sp))...)
	}
	return locs
}

// semanticReferences collects span-based references to a symbol across all
// loaded modules. These come from import declarations, OID parent references,
// and index entries.
func semanticReferences(m *mib.Mib, name string) []protocol.Location {
	var locs []protocol.Location

	// Import symbol references: use ModulesImporting to narrow the search.
	for _, mod := range m.ModulesImporting(name) {
		sp := mod.SourcePath()
		if sp == "" {
			continue
		}
		uri := pathToURI(sp)
		for _, imp := range mod.Imports() {
			for _, sym := range imp.Symbols {
				if sym.Name == name {
					if r, ok := spanToRange(mod, sym.Span); ok {
						locs = append(locs, protocol.Location{URI: uri, Range: r})
					}
				}
			}
		}
	}

	// OID parent references and index entry references.
	for _, mod := range m.Modules() {
		sp := mod.SourcePath()
		if sp == "" {
			continue
		}
		uri := pathToURI(sp)

		collectOidRefLocs(&locs, mod, mod.Objects(), name, uri)
		collectOidRefLocs(&locs, mod, mod.Notifications(), name, uri)
		collectOidRefLocs(&locs, mod, mod.Groups(), name, uri)
		collectOidRefLocs(&locs, mod, mod.Compliances(), name, uri)
		collectOidRefLocs(&locs, mod, mod.Capabilities(), name, uri)

		for _, obj := range mod.Objects() {
			for _, idx := range obj.Index() {
				if idx.Object != nil && idx.Object.Name() == name {
					if r, ok := spanToRange(mod, idx.Span); ok {
						locs = append(locs, protocol.Location{URI: uri, Range: r})
					}
				}
			}
		}
	}

	return locs
}

// oidRefer is implemented by entity types that expose OID references.
type oidRefer interface {
	OidRefs() []mib.OidRef
}

// collectOidRefLocs appends locations for OidRef spans matching name.
func collectOidRefLocs[T oidRefer](locs *[]protocol.Location, mod *mib.Module, entities []T, name string, uri protocol.DocumentUri) {
	for _, e := range entities {
		for _, ref := range e.OidRefs() {
			if ref.Name == name {
				if r, ok := spanToRange(mod, ref.Span); ok {
					*locs = append(*locs, protocol.Location{URI: uri, Range: r})
				}
			}
		}
	}
}

// mergeLocations merges extra locations into base, deduplicating by position.
func mergeLocations(base, extra []protocol.Location) []protocol.Location {
	type key struct {
		uri  protocol.DocumentUri
		line protocol.UInteger
		col  protocol.UInteger
	}
	seen := make(map[key]struct{}, len(base))
	for _, loc := range base {
		seen[key{loc.URI, loc.Range.Start.Line, loc.Range.Start.Character}] = struct{}{}
	}
	for _, loc := range extra {
		k := key{loc.URI, loc.Range.Start.Line, loc.Range.Start.Character}
		if _, ok := seen[k]; !ok {
			base = append(base, loc)
			seen[k] = struct{}{}
		}
	}
	return base
}

// excludeDefinition removes the location matching the symbol's definition.
func excludeDefinition(locs []protocol.Location, m *mib.Mib, name string) []protocol.Location {
	mod, span := symbolDefinition(m, name)
	if mod == nil || mod.SourcePath() == "" {
		return locs
	}

	defLine, defCol := mod.LineCol(span.Start)
	if defLine == 0 {
		return locs
	}

	defURI := pathToURI(mod.SourcePath())
	lspLine := protocol.UInteger(defLine - 1)
	lspCol := protocol.UInteger(defCol - 1)

	filtered := make([]protocol.Location, 0, len(locs))
	for _, loc := range locs {
		if loc.URI == defURI && loc.Range.Start.Line == lspLine && loc.Range.Start.Character == lspCol {
			continue
		}
		filtered = append(filtered, loc)
	}
	return filtered
}
