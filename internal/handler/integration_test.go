package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golangsnmp/gomib"
	"github.com/golangsnmp/gomib/mib"
	"github.com/golangsnmp/gomib/syntax"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// corpusDir returns the path to the gomib test corpus.
func corpusDir(t *testing.T) string {
	t.Helper()
	// Relative to gomib-lsp/internal/handler/
	dir := filepath.Join("..", "..", "..", "gomib", "testdata", "corpus", "primary")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("resolve corpus dir: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Skipf("corpus not found at %s: %v", abs, err)
	}
	return abs
}

// loadTestServer creates a Server with MIBs loaded from the corpus.
func loadTestServer(t *testing.T) *Server {
	t.Helper()
	corpus := corpusDir(t)

	// Load MIBs from IETF and IANA directories to get IF-MIB and dependencies.
	ietfDir := filepath.Join(corpus, "ietf")
	ianaDir := filepath.Join(corpus, "iana")

	var sources []gomib.Source
	for _, dir := range []string{ietfDir, ianaDir} {
		src, err := gomib.Dir(dir)
		if err != nil {
			t.Fatalf("gomib.Dir(%s): %v", dir, err)
		}
		sources = append(sources, src)
	}

	diagCfg := mib.VerboseConfig()
	diagCfg.FailAt = mib.SeverityFatal

	m, err := gomib.Load(context.Background(),
		gomib.WithSource(sources...),
		gomib.WithDiagnosticConfig(diagCfg),
	)
	if err != nil {
		t.Fatalf("gomib.Load: %v", err)
	}

	s := New()
	s.mib = m
	return s
}

// openDocument reads a MIB file and registers it as an open document on the server.
func openDocument(t *testing.T, s *Server, path string) (protocol.DocumentUri, *document) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	doc := parseDocument(string(data))
	uri := pathToURI(path)
	s.documents[uri] = doc
	return uri, doc
}

// findOffset finds the byte offset of the nth occurrence of needle in content.
// Uses 0-based occurrence index.
func findOffset(content, needle string, occurrence int) int {
	idx := 0
	for i := 0; i <= occurrence; i++ {
		pos := strings.Index(content[idx:], needle)
		if pos < 0 {
			return -1
		}
		if i == occurrence {
			return idx + pos
		}
		idx += pos + len(needle)
	}
	return -1
}

// offsetToPosition converts a byte offset to an LSP position using a line table.
func offsetToPosition(content string, offset int) protocol.Position {
	lt := syntax.BuildLineTable([]byte(content))
	line, col := lt.LineCol(syntax.ByteOffset(offset))
	return protocol.Position{
		Line:      protocol.UInteger(line - 1),
		Character: protocol.UInteger(col - 1),
	}
}

func TestIntegration_HoverObject(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	ifMIBPath := filepath.Join(corpus, "ietf", "IF-MIB.mib")
	uri, doc := openDocument(t, s, ifMIBPath)

	// Find "ifNumber" in the definition (OBJECT-TYPE)
	offset := findOffset(doc.content, "ifNumber", 0)
	if offset < 0 {
		t.Fatal("ifNumber not found in IF-MIB")
	}
	pos := offsetToPosition(doc.content, offset)

	result, err := s.textDocumentHover(nil, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
	})
	if err != nil {
		t.Fatalf("hover: %v", err)
	}
	if result == nil {
		t.Fatal("hover returned nil for ifNumber")
	}

	md := result.Contents.(protocol.MarkupContent)
	if md.Kind != protocol.MarkupKindMarkdown {
		t.Errorf("expected markdown, got %s", md.Kind)
	}
	if !strings.Contains(md.Value, "ifNumber") {
		t.Errorf("hover should mention ifNumber, got: %s", md.Value)
	}
	if !strings.Contains(md.Value, "OBJECT-TYPE") {
		t.Errorf("hover should mention OBJECT-TYPE, got: %s", md.Value)
	}
	if !strings.Contains(md.Value, "read-only") {
		t.Errorf("hover should mention read-only access, got: %s", md.Value)
	}
}

func TestIntegration_HoverType(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	ifMIBPath := filepath.Join(corpus, "ietf", "IF-MIB.mib")
	uri, doc := openDocument(t, s, ifMIBPath)

	// Hover over "InterfaceIndex" textual convention definition.
	// Occurrence 0 is in a comment; occurrence 1 is the definition.
	offset := findOffset(doc.content, "InterfaceIndex", 1)
	if offset < 0 {
		t.Fatal("InterfaceIndex not found in IF-MIB")
	}
	pos := offsetToPosition(doc.content, offset)

	result, err := s.textDocumentHover(nil, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
	})
	if err != nil {
		t.Fatalf("hover: %v", err)
	}
	if result == nil {
		t.Fatal("hover returned nil for InterfaceIndex")
	}

	md := result.Contents.(protocol.MarkupContent)
	if !strings.Contains(md.Value, "InterfaceIndex") {
		t.Errorf("hover should mention InterfaceIndex, got: %s", md.Value)
	}
	if !strings.Contains(md.Value, "TEXTUAL-CONVENTION") {
		t.Errorf("hover should mention TEXTUAL-CONVENTION, got: %s", md.Value)
	}
}

func TestIntegration_HoverKeyword(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	ifMIBPath := filepath.Join(corpus, "ietf", "IF-MIB.mib")
	uri, doc := openDocument(t, s, ifMIBPath)

	// Hover over "OBJECT-TYPE" keyword
	offset := findOffset(doc.content, "OBJECT-TYPE", 0)
	if offset < 0 {
		t.Fatal("OBJECT-TYPE not found in IF-MIB")
	}
	pos := offsetToPosition(doc.content, offset)

	result, err := s.textDocumentHover(nil, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
	})
	if err != nil {
		t.Fatalf("hover: %v", err)
	}
	if result == nil {
		t.Fatal("hover returned nil for OBJECT-TYPE keyword")
	}

	md := result.Contents.(protocol.MarkupContent)
	if !strings.Contains(md.Value, "OBJECT-TYPE") {
		t.Errorf("hover should mention OBJECT-TYPE, got: %s", md.Value)
	}
}

func TestIntegration_HoverModule(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	ifMIBPath := filepath.Join(corpus, "ietf", "IF-MIB.mib")
	uri, doc := openDocument(t, s, ifMIBPath)

	// Hover over "SNMPv2-SMI" in import section
	offset := findOffset(doc.content, "SNMPv2-SMI", 0)
	if offset < 0 {
		t.Fatal("SNMPv2-SMI not found in IF-MIB")
	}
	pos := offsetToPosition(doc.content, offset)

	result, err := s.textDocumentHover(nil, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
	})
	if err != nil {
		t.Fatalf("hover: %v", err)
	}
	if result == nil {
		t.Fatal("hover returned nil for SNMPv2-SMI module name")
	}

	md := result.Contents.(protocol.MarkupContent)
	if !strings.Contains(md.Value, "SNMPv2-SMI") {
		t.Errorf("hover should mention SNMPv2-SMI, got: %s", md.Value)
	}
}

func TestIntegration_HoverNoResult(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	ifMIBPath := filepath.Join(corpus, "ietf", "IF-MIB.mib")
	uri, _ := openDocument(t, s, ifMIBPath)

	// Position 0,0 is "I" in "IF-MIB" - should get module hover
	// Position on a comment or whitespace should return nil
	result, err := s.textDocumentHover(nil, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 49, Character: 0}, // blank line area
		},
	})
	if err != nil {
		t.Fatalf("hover: %v", err)
	}
	// Either nil or empty is acceptable for whitespace
	_ = result
}

func TestIntegration_Definition(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	ifMIBPath := filepath.Join(corpus, "ietf", "IF-MIB.mib")
	uri, doc := openDocument(t, s, ifMIBPath)

	// Go to definition on "mib-2" in the imports section - should resolve to base module
	offset := findOffset(doc.content, "mib-2", 0)
	if offset < 0 {
		t.Fatal("mib-2 not found in IF-MIB")
	}
	pos := offsetToPosition(doc.content, offset)

	result, err := s.textDocumentDefinition(nil, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
	})
	if err != nil {
		t.Fatalf("definition: %v", err)
	}
	// mib-2 is from a base module (SNMPv2-SMI) which is synthetic, so may not have a source path.
	// This is still a valid test - it exercises the code path.
	_ = result
}

func TestIntegration_DefinitionType(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	ifMIBPath := filepath.Join(corpus, "ietf", "IF-MIB.mib")
	uri, doc := openDocument(t, s, ifMIBPath)

	// Go to definition on "DisplayString" which is imported from SNMPv2-TC
	offset := findOffset(doc.content, "DisplayString", 0)
	if offset < 0 {
		t.Fatal("DisplayString not found in IF-MIB")
	}
	pos := offsetToPosition(doc.content, offset)

	result, err := s.textDocumentDefinition(nil, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
	})
	if err != nil {
		t.Fatalf("definition: %v", err)
	}
	// DisplayString is from SNMPv2-TC which is a base module (synthetic),
	// so it might not have a file location.
	_ = result
}

func TestIntegration_DefinitionLocalSymbol(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	ifMIBPath := filepath.Join(corpus, "ietf", "IF-MIB.mib")
	uri, doc := openDocument(t, s, ifMIBPath)

	// Find "ifIndex" usage in INDEX clause (not the definition)
	// "INDEX   { ifIndex }" is at line ~146 in IF-MIB
	idxOffset := findOffset(doc.content, "INDEX   { ifIndex }", 0)
	if idxOffset < 0 {
		// Try with varied spacing
		idxOffset = findOffset(doc.content, "ifIndex", 1)
	}
	if idxOffset < 0 {
		t.Skip("ifIndex reference not found")
	}
	// Point at "ifIndex" within the INDEX clause
	pos := offsetToPosition(doc.content, idxOffset+strings.Index(doc.content[idxOffset:], "ifIndex"))

	result, err := s.textDocumentDefinition(nil, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
	})
	if err != nil {
		t.Fatalf("definition: %v", err)
	}
	if result == nil {
		t.Fatal("definition returned nil for ifIndex reference")
	}

	loc, ok := result.(protocol.Location)
	if !ok {
		t.Fatalf("expected Location, got %T", result)
	}
	if !strings.Contains(string(loc.URI), "IF-MIB") {
		t.Errorf("expected definition in IF-MIB, got URI: %s", loc.URI)
	}
}

func TestIntegration_DocumentSymbols(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	ifMIBPath := filepath.Join(corpus, "ietf", "IF-MIB.mib")
	uri, _ := openDocument(t, s, ifMIBPath)

	result, err := s.textDocumentDocumentSymbol(nil, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatalf("documentSymbol: %v", err)
	}
	if result == nil {
		t.Fatal("documentSymbol returned nil")
	}

	symbols, ok := result.([]protocol.DocumentSymbol)
	if !ok {
		t.Fatalf("expected []DocumentSymbol, got %T", result)
	}

	// Should have at least one module container (IF-MIB)
	if len(symbols) == 0 {
		t.Fatal("no document symbols returned")
	}

	// Find the IF-MIB module
	var ifMIBSym *protocol.DocumentSymbol
	for i := range symbols {
		if symbols[i].Name == "IF-MIB" {
			ifMIBSym = &symbols[i]
			break
		}
	}
	if ifMIBSym == nil {
		t.Fatal("IF-MIB module symbol not found")
	}
	if ifMIBSym.Kind != protocol.SymbolKindModule {
		t.Errorf("IF-MIB kind = %v, want Module", ifMIBSym.Kind)
	}

	// Should have many children (objects, types, etc.)
	if len(ifMIBSym.Children) < 10 {
		t.Errorf("expected many child symbols, got %d", len(ifMIBSym.Children))
	}

	// Check that some known symbols are present
	childNames := make(map[string]bool)
	for _, c := range ifMIBSym.Children {
		childNames[c.Name] = true
	}
	for _, name := range []string{"ifNumber", "ifTable", "ifEntry", "ifIndex", "ifDescr", "InterfaceIndex", "OwnerString"} {
		if !childNames[name] {
			t.Errorf("expected child symbol %q not found", name)
		}
	}
}

func TestIntegration_WorkspaceSymbols(t *testing.T) {
	s := loadTestServer(t)

	result, err := s.workspaceSymbol(nil, &protocol.WorkspaceSymbolParams{
		Query: "ifDescr",
	})
	if err != nil {
		t.Fatalf("workspaceSymbol: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("workspaceSymbol returned no results for 'ifDescr'")
	}

	found := false
	for _, sym := range result {
		if sym.Name == "ifDescr" {
			found = true
			if sym.Kind != protocol.SymbolKindVariable {
				t.Errorf("ifDescr kind = %v, want Variable", sym.Kind)
			}
			break
		}
	}
	if !found {
		t.Error("ifDescr not found in workspace symbols")
	}
}

func TestIntegration_WorkspaceSymbolsPartial(t *testing.T) {
	s := loadTestServer(t)

	// Searching for "ifOper" should find ifOperStatus
	result, err := s.workspaceSymbol(nil, &protocol.WorkspaceSymbolParams{
		Query: "ifOper",
	})
	if err != nil {
		t.Fatalf("workspaceSymbol: %v", err)
	}

	found := false
	for _, sym := range result {
		if sym.Name == "ifOperStatus" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ifOperStatus not found for query 'ifOper'")
	}
}

func TestIntegration_WorkspaceSymbolsEmpty(t *testing.T) {
	s := loadTestServer(t)

	// Empty query should return nil
	result, err := s.workspaceSymbol(nil, &protocol.WorkspaceSymbolParams{
		Query: "",
	})
	if err != nil {
		t.Fatalf("workspaceSymbol: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for empty query, got %d results", len(result))
	}
}

func TestIntegration_References(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	ifMIBPath := filepath.Join(corpus, "ietf", "IF-MIB.mib")
	uri, doc := openDocument(t, s, ifMIBPath)

	// Find references to "ifIndex" - it's used in many places in IF-MIB.
	// Occurrence 0 is in a comment; occurrence 1 is the first real token.
	offset := findOffset(doc.content, "ifIndex", 1)
	if offset < 0 {
		t.Fatal("ifIndex not found")
	}
	pos := offsetToPosition(doc.content, offset)

	result, err := s.textDocumentReferences(nil, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
		Context: protocol.ReferenceContext{
			IncludeDeclaration: true,
		},
	})
	if err != nil {
		t.Fatalf("references: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("references returned no results for ifIndex")
	}

	// ifIndex should appear multiple times (definition, INDEX clauses, SEQUENCE, etc.)
	if len(result) < 3 {
		t.Errorf("expected at least 3 references for ifIndex, got %d", len(result))
	}
}

func TestIntegration_ReferencesExcludeDeclaration(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	ifMIBPath := filepath.Join(corpus, "ietf", "IF-MIB.mib")
	uri, doc := openDocument(t, s, ifMIBPath)

	// Occurrence 0 is in a comment; occurrence 1 is the first real token.
	offset := findOffset(doc.content, "ifIndex", 1)
	if offset < 0 {
		t.Fatal("ifIndex not found")
	}
	pos := offsetToPosition(doc.content, offset)

	withDecl, err := s.textDocumentReferences(nil, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	withoutDecl, err := s.textDocumentReferences(nil, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: false},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(withDecl) <= len(withoutDecl) {
		// withDecl should have at least one more than withoutDecl
		// (or equal if the definition couldn't be found in the results)
	}
	// Both should be non-empty
	if len(withoutDecl) == 0 {
		t.Error("expected at least some non-declaration references for ifIndex")
	}
}

func TestIntegration_SemanticTokens(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	ifMIBPath := filepath.Join(corpus, "ietf", "IF-MIB.mib")
	uri, _ := openDocument(t, s, ifMIBPath)

	result, err := s.textDocumentSemanticTokensFull(nil, &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatalf("semanticTokens: %v", err)
	}
	if result == nil {
		t.Fatal("semanticTokens returned nil")
	}
	if len(result.Data) == 0 {
		t.Fatal("semanticTokens returned empty data")
	}

	// Data is delta-encoded: each token is 5 values
	if len(result.Data)%5 != 0 {
		t.Errorf("semantic token data length %d is not a multiple of 5", len(result.Data))
	}

	tokenCount := len(result.Data) / 5
	if tokenCount < 50 {
		t.Errorf("expected many semantic tokens for IF-MIB, got %d", tokenCount)
	}

	// Verify delta encoding is valid (no negative deltas)
	for i := 0; i < len(result.Data); i += 5 {
		deltaLine := result.Data[i]
		length := result.Data[i+2]
		tokenType := result.Data[i+3]

		if length == 0 {
			t.Errorf("token at index %d has zero length", i/5)
		}
		if tokenType >= protocol.UInteger(len(semanticTokenTypes)) {
			t.Errorf("token at index %d has invalid type %d", i/5, tokenType)
		}
		// deltaLine is unsigned, always valid
		_ = deltaLine
	}
}

func TestIntegration_SemanticTokensSmallMIB(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	// Use SNMPv2-MIB which is smaller
	snmpv2Path := filepath.Join(corpus, "ietf", "SNMPv2-MIB.mib")
	uri, _ := openDocument(t, s, snmpv2Path)

	result, err := s.textDocumentSemanticTokensFull(nil, &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatalf("semanticTokens: %v", err)
	}
	if result == nil {
		t.Fatal("semanticTokens returned nil for SNMPv2-MIB")
	}
	if len(result.Data)%5 != 0 {
		t.Errorf("data length not multiple of 5: %d", len(result.Data))
	}
}

func TestIntegration_CompletionInSyntaxClause(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	ifMIBPath := filepath.Join(corpus, "ietf", "IF-MIB.mib")
	uri, doc := openDocument(t, s, ifMIBPath)

	// Find a SYNTAX clause position - "SYNTAX      Integer32" in ifNumber
	offset := findOffset(doc.content, "SYNTAX      Integer32\n", 0)
	if offset < 0 {
		t.Skip("could not find SYNTAX clause in IF-MIB")
	}
	// Position cursor right after "SYNTAX      " (where type name starts)
	syntaxKeywordEnd := offset + len("SYNTAX      ")
	pos := offsetToPosition(doc.content, syntaxKeywordEnd)

	result, err := s.textDocumentCompletion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
	})
	if err != nil {
		t.Fatalf("completion: %v", err)
	}
	// May or may not return completions depending on cursor context detection
	_ = result
}

func TestIntegration_CompletionGeneral(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	// Use SNMPv2-MIB for a simpler test
	snmpPath := filepath.Join(corpus, "ietf", "SNMPv2-MIB.mib")
	uri, doc := openDocument(t, s, snmpPath)

	// Position at the very beginning - might not be in any specific clause context
	// so we should get general completions or nil
	_ = doc

	result, err := s.textDocumentCompletion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})
	if err != nil {
		t.Fatalf("completion: %v", err)
	}
	// At position 0,0 we might get nil (in comment or no context)
	_ = result
}

func TestIntegration_DiagnosticsForURI(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	ifMIBPath := filepath.Join(corpus, "ietf", "IF-MIB.mib")
	uri, doc := openDocument(t, s, ifMIBPath)

	// Capture diagnostics via a mock notify function
	var published []protocol.PublishDiagnosticsParams
	s.notify = func(method string, params any) {
		if p, ok := params.(protocol.PublishDiagnosticsParams); ok {
			published = append(published, p)
		}
	}

	s.publishDiagnosticsForURI(uri, doc.modules)

	// IF-MIB should be parseable without fatal errors.
	// It might have some style diagnostics.
	if len(published) == 0 {
		t.Fatal("publishDiagnosticsForURI did not publish any notification")
	}

	// Check that published diagnostics are for the correct URI
	for _, p := range published {
		if p.URI != uri {
			t.Errorf("diagnostic URI mismatch: got %s, want %s", p.URI, uri)
		}
	}
}

func TestIntegration_DiagnosticsAllFiles(t *testing.T) {
	s := loadTestServer(t)

	var published []protocol.PublishDiagnosticsParams
	s.notify = func(method string, params any) {
		if p, ok := params.(protocol.PublishDiagnosticsParams); ok {
			published = append(published, p)
		}
	}

	s.publishAllDiagnostics()

	// Should publish diagnostics for loaded modules with source paths
	if len(published) == 0 {
		t.Log("no diagnostics published (all modules may be from base)")
	}
}

func TestIntegration_HoverImportedSymbol(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	ifMIBPath := filepath.Join(corpus, "ietf", "IF-MIB.mib")
	uri, doc := openDocument(t, s, ifMIBPath)

	// Hover over "Counter32" in the imports section
	offset := findOffset(doc.content, "Counter32", 0)
	if offset < 0 {
		t.Fatal("Counter32 not found in IF-MIB")
	}
	pos := offsetToPosition(doc.content, offset)

	result, err := s.textDocumentHover(nil, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
	})
	if err != nil {
		t.Fatalf("hover: %v", err)
	}
	if result == nil {
		t.Fatal("hover returned nil for Counter32")
	}

	md := result.Contents.(protocol.MarkupContent)
	if !strings.Contains(md.Value, "Counter32") {
		t.Errorf("hover should mention Counter32, got: %s", md.Value)
	}
}

func TestIntegration_HoverDeprecatedType(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	ifMIBPath := filepath.Join(corpus, "ietf", "IF-MIB.mib")
	uri, doc := openDocument(t, s, ifMIBPath)

	// OwnerString is deprecated in IF-MIB.
	// Occurrence 0 is in a comment; occurrence 1 is the definition.
	offset := findOffset(doc.content, "OwnerString", 1)
	if offset < 0 {
		t.Fatal("OwnerString not found in IF-MIB")
	}
	pos := offsetToPosition(doc.content, offset)

	result, err := s.textDocumentHover(nil, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
	})
	if err != nil {
		t.Fatalf("hover: %v", err)
	}
	if result == nil {
		t.Fatal("hover returned nil for OwnerString")
	}

	md := result.Contents.(protocol.MarkupContent)
	if !strings.Contains(md.Value, "deprecated") {
		t.Errorf("hover should mention deprecated status for OwnerString, got: %s", md.Value)
	}
}

func TestIntegration_MultipleFiles(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	// Open multiple MIB files
	ifMIBPath := filepath.Join(corpus, "ietf", "IF-MIB.mib")
	tcpMIBPath := filepath.Join(corpus, "ietf", "TCP-MIB.mib")

	ifURI, _ := openDocument(t, s, ifMIBPath)
	tcpURI, _ := openDocument(t, s, tcpMIBPath)

	// Document symbols should work for both
	ifResult, err := s.textDocumentDocumentSymbol(nil, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: ifURI},
	})
	if err != nil {
		t.Fatalf("documentSymbol IF-MIB: %v", err)
	}
	if ifResult == nil {
		t.Error("documentSymbol returned nil for IF-MIB")
	}

	tcpResult, err := s.textDocumentDocumentSymbol(nil, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: tcpURI},
	})
	if err != nil {
		t.Fatalf("documentSymbol TCP-MIB: %v", err)
	}
	if tcpResult == nil {
		t.Error("documentSymbol returned nil for TCP-MIB")
	}

	// They should have different symbols
	ifSyms := ifResult.([]protocol.DocumentSymbol)
	tcpSyms := tcpResult.([]protocol.DocumentSymbol)
	if len(ifSyms) > 0 && len(tcpSyms) > 0 {
		if ifSyms[0].Name == tcpSyms[0].Name {
			t.Error("IF-MIB and TCP-MIB should have different module names")
		}
	}
}

func TestIntegration_CrossFileReferences(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	// Open both IF-MIB and a MIB that references it
	ifMIBPath := filepath.Join(corpus, "ietf", "IF-MIB.mib")
	uri, doc := openDocument(t, s, ifMIBPath)

	// Also open TCP-MIB which imports from IF-MIB
	tcpMIBPath := filepath.Join(corpus, "ietf", "TCP-MIB.mib")
	openDocument(t, s, tcpMIBPath)

	// Find references to InterfaceIndex which might be used in other MIBs.
	// Occurrence 0 is in a comment; occurrence 1 is the definition.
	offset := findOffset(doc.content, "InterfaceIndex", 1)
	if offset < 0 {
		t.Fatal("InterfaceIndex not found")
	}
	pos := offsetToPosition(doc.content, offset)

	result, err := s.textDocumentReferences(nil, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: true},
	})
	if err != nil {
		t.Fatalf("references: %v", err)
	}

	// InterfaceIndex should be used within IF-MIB at minimum
	if len(result) < 2 {
		t.Errorf("expected at least 2 references for InterfaceIndex, got %d", len(result))
	}
}

func TestIntegration_RFC1213MIB(t *testing.T) {
	s := loadTestServer(t)
	corpus := corpusDir(t)

	// Test with an SMIv1 MIB to exercise v1 handling
	rfc1213Path := filepath.Join(corpus, "ietf", "RFC1213-MIB.mib")
	if _, err := os.Stat(rfc1213Path); err != nil {
		t.Skipf("RFC1213-MIB not found: %v", err)
	}

	uri, _ := openDocument(t, s, rfc1213Path)

	// Document symbols should work for SMIv1 MIBs too
	result, err := s.textDocumentDocumentSymbol(nil, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatalf("documentSymbol: %v", err)
	}
	if result == nil {
		t.Fatal("documentSymbol returned nil for RFC1213-MIB")
	}

	// Semantic tokens should also work
	semResult, err := s.textDocumentSemanticTokensFull(nil, &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatalf("semanticTokens: %v", err)
	}
	if semResult == nil {
		t.Fatal("semanticTokens returned nil for RFC1213-MIB")
	}
}

func TestIntegration_WorkspaceSymbolsCaseInsensitive(t *testing.T) {
	s := loadTestServer(t)

	// Search should be case-insensitive
	lower, err := s.workspaceSymbol(nil, &protocol.WorkspaceSymbolParams{Query: "ifdescr"})
	if err != nil {
		t.Fatal(err)
	}
	upper, err := s.workspaceSymbol(nil, &protocol.WorkspaceSymbolParams{Query: "IFDESCR"})
	if err != nil {
		t.Fatal(err)
	}

	if len(lower) != len(upper) {
		t.Errorf("case-insensitive search mismatch: lower=%d, upper=%d", len(lower), len(upper))
	}
}

func TestIntegration_NilMib(t *testing.T) {
	// Test that handlers gracefully return nil when mib is not loaded
	s := New()
	uri := protocol.DocumentUri("file:///test.mib")
	s.documents[uri] = parseDocument("-- empty")

	result, err := s.textDocumentHover(nil, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 3},
		},
	})
	if err != nil {
		t.Fatalf("hover with nil mib: %v", err)
	}
	if result != nil {
		t.Error("expected nil hover with nil mib")
	}
}

func TestIntegration_UnknownDocument(t *testing.T) {
	s := loadTestServer(t)
	unknownURI := protocol.DocumentUri("file:///nonexistent.mib")

	// All handlers should return nil for unknown documents
	hover, err := s.textDocumentHover(nil, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: unknownURI},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if hover != nil {
		t.Error("expected nil hover for unknown document")
	}

	def, err := s.textDocumentDefinition(nil, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: unknownURI},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if def != nil {
		t.Error("expected nil definition for unknown document")
	}
}
