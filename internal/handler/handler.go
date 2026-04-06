package handler

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/golangsnmp/gomib"
	"github.com/golangsnmp/gomib/mib"
	"github.com/golangsnmp/gomib/syntax"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// document tracks an open file's content and cached analysis.
type document struct {
	content   string
	modules   []string // module names extracted from content
	cst       *syntax.ModuleFile
	lineTable syntax.LineTable
}

// Server holds per-session state for the LSP server.
type Server struct {
	Handler protocol.Handler

	mu             sync.Mutex
	documents      map[string]*document // URI -> open document
	workspaceRoots []string
	mib            *mib.Mib
	diagnosticURIs map[protocol.DocumentUri]struct{} // URIs with published diagnostics

	debounceMu    sync.Mutex
	debounceTimer *time.Timer

	notify glsp.NotifyFunc
}

// New creates a Server with all handler methods wired.
func New() *Server {
	s := &Server{
		documents:      make(map[string]*document),
		diagnosticURIs: make(map[protocol.DocumentUri]struct{}),
	}

	s.Handler.Initialize = s.initialize
	s.Handler.Initialized = s.initialized
	s.Handler.Shutdown = s.shutdown
	s.Handler.SetTrace = s.setTrace
	s.Handler.TextDocumentDidOpen = s.textDocumentDidOpen
	s.Handler.TextDocumentDidChange = s.textDocumentDidChange
	s.Handler.TextDocumentDidClose = s.textDocumentDidClose
	s.Handler.TextDocumentDidSave = s.textDocumentDidSave
	s.Handler.TextDocumentHover = s.textDocumentHover
	s.Handler.TextDocumentDefinition = s.textDocumentDefinition
	s.Handler.TextDocumentDocumentSymbol = s.textDocumentDocumentSymbol
	s.Handler.TextDocumentReferences = s.textDocumentReferences
	s.Handler.WorkspaceSymbol = s.workspaceSymbol
	s.Handler.TextDocumentCompletion = s.textDocumentCompletion
	s.Handler.TextDocumentSemanticTokensFull = s.textDocumentSemanticTokensFull

	return s
}

func (s *Server) initialize(ctx *glsp.Context, params *protocol.InitializeParams) (any, error) {
	capabilities := s.Handler.CreateServerCapabilities()
	capabilities.TextDocumentSync = protocol.TextDocumentSyncKindFull
	capabilities.SemanticTokensProvider = protocol.SemanticTokensOptions{
		Legend: semanticTokensLegend(),
		Full:   true,
	}

	// Extract workspace root paths.
	var roots []string
	for _, folder := range params.WorkspaceFolders {
		if p := uriToPath(folder.URI); p != "" {
			roots = append(roots, p)
		}
	}
	if len(roots) == 0 && params.RootURI != nil {
		if p := uriToPath(*params.RootURI); p != "" {
			roots = append(roots, p)
		}
	}
	if len(roots) == 0 && params.RootPath != nil && *params.RootPath != "" {
		roots = append(roots, *params.RootPath)
	}
	s.mu.Lock()
	s.workspaceRoots = roots
	s.mu.Unlock()

	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    "mib-lsp",
			Version: ptrTo("0.1.0"),
		},
	}, nil
}

func (s *Server) initialized(ctx *glsp.Context, params *protocol.InitializedParams) error {
	s.notify = ctx.Notify
	s.loadWorkspace()
	s.publishAllDiagnostics()
	return nil
}

func (s *Server) shutdown(ctx *glsp.Context) error {
	s.debounceMu.Lock()
	if s.debounceTimer != nil {
		s.debounceTimer.Stop()
	}
	s.debounceMu.Unlock()
	return nil
}

func (s *Server) setTrace(ctx *glsp.Context, params *protocol.SetTraceParams) error {
	return nil
}

func (s *Server) textDocumentDidOpen(ctx *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	uri := params.TextDocument.URI
	content := params.TextDocument.Text
	doc := parseDocument(content)

	s.mu.Lock()
	s.documents[uri] = doc
	needLoad := len(s.workspaceRoots) == 0
	s.mu.Unlock()

	// If no workspace roots were provided (single-file open), use the file's
	// parent directory and trigger a full workspace load.
	if needLoad {
		if p := uriToPath(uri); p != "" {
			dir := filepath.Dir(p)
			s.mu.Lock()
			s.workspaceRoots = []string{dir}
			s.mu.Unlock()
			s.loadWorkspace()
			s.publishAllDiagnostics()
			return nil
		}
	}

	s.publishDiagnosticsForURI(uri, doc.modules)
	return nil
}

func (s *Server) textDocumentDidChange(ctx *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	uri := params.TextDocument.URI
	s.mu.Lock()
	for _, change := range params.ContentChanges {
		if c, ok := change.(protocol.TextDocumentContentChangeEventWhole); ok {
			s.documents[uri] = parseDocument(c.Text)
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *Server) textDocumentDidSave(ctx *glsp.Context, params *protocol.DidSaveTextDocumentParams) error {
	s.scheduleReload()
	return nil
}

func (s *Server) textDocumentDidClose(ctx *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	uri := params.TextDocument.URI
	s.mu.Lock()
	delete(s.documents, uri)
	s.mu.Unlock()
	s.notifyDiagnostics(uri, nil)
	return nil
}

// parseDocument creates a document from content, parsing CST and building
// a line table for position conversion.
func parseDocument(content string) *document {
	src := []byte(content)
	cst, _ := syntax.Parse(src)
	return &document{
		content:   content,
		modules:   gomib.ScanModuleNames(src),
		cst:       cst,
		lineTable: syntax.BuildLineTable(src),
	}
}

func ptrTo[T any](v T) *T { return &v }
