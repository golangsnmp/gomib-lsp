package handler

import (
	lspuri "go.lsp.dev/uri"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// pathToURI converts a filesystem path to a file:// URI.
//
// Delegates to go.lsp.dev/uri, which handles Windows drive letters, URL
// encoding, and UNC paths per the LSP spec.
func pathToURI(path string) protocol.DocumentUri {
	return protocol.DocumentUri(lspuri.File(path))
}

// uriToPath converts a file:// URI to a filesystem path.
//
// Returns an empty string for non-file URIs or unparseable input.
func uriToPath(uri protocol.DocumentUri) string {
	u, err := lspuri.Parse(string(uri))
	if err != nil {
		return ""
	}
	return u.Filename()
}
