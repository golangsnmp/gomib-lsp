package lsp

import (
	"github.com/tliron/glsp/server"

	"github.com/golangsnmp/gomib-lsp/internal/handler"
)

const ServerName = "mib-lsp"

// Run creates the GLSP server and runs it over stdio.
func Run() error {
	h := handler.New()
	s := server.NewServer(&h.Handler, ServerName, false)
	return s.RunStdio()
}
