package main

import (
	"fmt"
	"os"

	"github.com/golangsnmp/gomib-lsp/internal/lsp"
)

func main() {
	if err := lsp.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "mib-lsp: %v\n", err)
		os.Exit(1)
	}
}
