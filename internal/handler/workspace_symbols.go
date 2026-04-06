package handler

import (
	"strings"

	"github.com/golangsnmp/gomib/mib"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) workspaceSymbol(ctx *glsp.Context, params *protocol.WorkspaceSymbolParams) ([]protocol.SymbolInformation, error) {
	if params.Query == "" {
		return nil, nil
	}

	s.mu.Lock()
	m := s.mib
	s.mu.Unlock()

	if m == nil {
		return nil, nil
	}

	query := strings.ToLower(params.Query)
	var results []protocol.SymbolInformation

	for _, mod := range m.Modules() {
		if mod.SourcePath() == "" {
			continue
		}

		uri := pathToURI(mod.SourcePath())
		modName := mod.Name()

		for def := range mod.Definitions() {
			kind := symbolKindFromMib(def.Kind())
			if sym, ok := workspaceSym(mod, def.Name(), def.Span(), kind, uri, modName, query); ok {
				results = append(results, sym)
			}
		}
	}

	return results, nil
}

func workspaceSym(mod *mib.Module, name string, span mib.Span, kind protocol.SymbolKind, uri protocol.DocumentUri, containerName, query string) (protocol.SymbolInformation, bool) {
	if name == "" || !strings.Contains(strings.ToLower(name), query) {
		return protocol.SymbolInformation{}, false
	}
	r, ok := spanToRange(mod, span)
	if !ok {
		return protocol.SymbolInformation{}, false
	}
	return protocol.SymbolInformation{
		Name:          name,
		Kind:          kind,
		Location:      protocol.Location{URI: uri, Range: r},
		ContainerName: ptrTo(containerName),
	}, true
}
