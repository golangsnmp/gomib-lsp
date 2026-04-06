package handler

import (
	"context"
	"net/url"
	"time"

	"github.com/golangsnmp/gomib"
	"github.com/golangsnmp/gomib/mib"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// loadWorkspace loads all MIBs from the workspace roots into a single Mib.
func (s *Server) loadWorkspace() {
	s.mu.Lock()
	roots := s.workspaceRoots
	s.mu.Unlock()

	if len(roots) == 0 {
		return
	}

	var sources []gomib.Source
	for _, root := range roots {
		src, err := gomib.Dir(root)
		if err != nil {
			continue
		}
		sources = append(sources, src)
	}
	if len(sources) == 0 {
		return
	}

	diagCfg := mib.VerboseConfig()
	diagCfg.FailAt = mib.SeverityFatal

	m, _ := gomib.Load(context.Background(),
		gomib.WithSource(sources...),
		gomib.WithSystemPaths(),
		gomib.WithDiagnosticConfig(diagCfg),
	)

	s.mu.Lock()
	s.mib = m
	s.mu.Unlock()
}

// scheduleReload debounces workspace reloads. Each call resets a 500ms timer;
// when it fires, the workspace is reloaded and diagnostics are republished.
func (s *Server) scheduleReload() {
	s.debounceMu.Lock()
	defer s.debounceMu.Unlock()

	if s.debounceTimer != nil {
		s.debounceTimer.Stop()
	}
	s.debounceTimer = time.AfterFunc(500*time.Millisecond, func() {
		s.loadWorkspace()
		s.publishAllDiagnostics()
	})
}

// publishDiagnosticsForURI pushes diagnostics for a single URI by filtering
// the workspace Mib's diagnostics to the given module names.
func (s *Server) publishDiagnosticsForURI(uri protocol.DocumentUri, moduleNames []string) {
	s.mu.Lock()
	m := s.mib
	s.mu.Unlock()

	if m == nil || len(moduleNames) == 0 {
		s.notifyDiagnostics(uri, nil)
		return
	}

	moduleSet := make(map[string]struct{}, len(moduleNames))
	for _, name := range moduleNames {
		moduleSet[name] = struct{}{}
	}

	diags := make([]protocol.Diagnostic, 0)
	for _, d := range m.Diagnostics() {
		if _, ok := moduleSet[d.Module]; !ok {
			continue
		}
		line := d.Line - 1
		col := d.Column - 1
		if line < 0 {
			line = 0
		}
		if col < 0 {
			col = 0
		}
		diags = append(diags, protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{Line: protocol.UInteger(line), Character: protocol.UInteger(col)},
				End:   protocol.Position{Line: protocol.UInteger(line), Character: protocol.UInteger(col)},
			},
			Severity: ptrTo(mapSeverity(d.Severity)),
			Code:     &protocol.IntegerOrString{Value: d.Code},
			Source:   ptrTo("gomib"),
			Message:  d.Message,
		})
	}

	s.notifyDiagnostics(uri, diags)
}

// publishAllDiagnostics pushes diagnostics for all workspace files that have
// issues and clears diagnostics for files that no longer have any.
func (s *Server) publishAllDiagnostics() {
	s.mu.Lock()
	m := s.mib
	s.mu.Unlock()

	if m == nil {
		return
	}

	// Build a map from source path to module names for all loaded modules.
	pathModules := map[string][]string{}
	for _, mod := range m.Modules() {
		sp := mod.SourcePath()
		if sp == "" {
			continue
		}
		pathModules[sp] = append(pathModules[sp], mod.Name())
	}

	// Publish diagnostics for each source file.
	published := make(map[protocol.DocumentUri]struct{})
	for path, moduleNames := range pathModules {
		uri := pathToURI(path)
		published[uri] = struct{}{}
		s.publishDiagnosticsForURI(uri, moduleNames)
	}

	// Clear diagnostics for URIs that were previously published but no longer
	// have any modules (e.g., file was removed or modules were renamed).
	s.mu.Lock()
	prev := s.diagnosticURIs
	s.diagnosticURIs = published
	s.mu.Unlock()

	for uri := range prev {
		if _, ok := published[uri]; !ok {
			s.notifyDiagnostics(uri, nil)
		}
	}
}

// notifyDiagnostics pushes a diagnostics notification for the given URI.
// A nil slice is sent as an empty list to clear diagnostics.
func (s *Server) notifyDiagnostics(uri protocol.DocumentUri, diags []protocol.Diagnostic) {
	if s.notify == nil {
		return
	}
	if diags == nil {
		diags = []protocol.Diagnostic{}
	}
	s.notify(protocol.ServerTextDocumentPublishDiagnostics, protocol.PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diags,
	})
}

// mapSeverity converts a gomib severity to an LSP diagnostic severity.
func mapSeverity(sev mib.Severity) protocol.DiagnosticSeverity {
	switch {
	case sev.AtLeast(mib.SeverityError):
		return protocol.DiagnosticSeverityError
	case sev.AtLeast(mib.SeverityStyle):
		return protocol.DiagnosticSeverityWarning
	default:
		return protocol.DiagnosticSeverityInformation
	}
}

// pathToURI converts a filesystem path to a file:// URI.
func pathToURI(path string) protocol.DocumentUri {
	u := url.URL{Scheme: "file", Path: path}
	return u.String()
}

// uriToPath converts a file:// URI to a filesystem path.
func uriToPath(uri protocol.DocumentUri) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	return u.Path
}
