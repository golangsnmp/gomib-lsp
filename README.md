# gomib-lsp

Language server for SNMP MIB files, built on [gomib](https://github.com/golangsnmp/gomib).

## Features

- **Hover** - documentation, OID, type info, access, status for any symbol
- **Go to definition** - jump to symbol definitions across modules, including imports, OID parents, and SYNTAX type references
- **Find references** - locate all uses of a symbol (imports, OID parents, INDEX clauses) across loaded modules
- **Completion** - context-aware suggestions for symbols, types, module names, STATUS/ACCESS values
- **Document symbols** - outline of all definitions in a file
- **Workspace symbols** - search across all loaded modules
- **Semantic tokens** - token-level syntax highlighting (keywords, types, identifiers, OIDs)
- **Diagnostics** - parse and resolve diagnostics from gomib

Handles both SMIv1 (RFC 1155/1212) and SMIv2 (RFC 2578) modules.

## Install

```
go install github.com/golangsnmp/gomib-lsp/cmd/mib-lsp@latest
```

## Usage

The server communicates over stdio using the Language Server Protocol. Point your editor's LSP client at the `mib-lsp` binary.

### VS Code

Use with an LSP client extension configured to run `mib-lsp` for MIB file types.

### Neovim (lspconfig)

```lua
vim.api.nvim_create_autocmd({"BufRead", "BufNewFile"}, {
  pattern = {"*.mib", "*.my"},
  callback = function() vim.bo.filetype = "mib" end,
})

vim.lsp.config("mib_lsp", {
  cmd = { "mib-lsp" },
  filetypes = { "mib" },
  root_markers = { ".git" },
})

vim.lsp.enable("mib_lsp")
```

## How it works

On initialization, the server scans workspace directories for MIB files and loads them through gomib's parse/resolve pipeline. This builds a resolved MIB model with full OID tree, type chains, and cross-module references.

Each open document is independently parsed into a lossless CST for position-aware queries (cursor context, token lookup). The resolved model provides semantic information (definitions, types, OIDs) while the CST provides source-level precision (exact token spans, comment/string detection).

## License

MIT
