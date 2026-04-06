# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-04-07

Initial release.

### Added

- LSP server over stdio, using gomib v0.11.0
- Hover with symbol documentation, OID info, type chains, import provenance
- Go to definition for symbols, imports, OID parents, SYNTAX type references
- Find references across all loaded modules (imports, OID parents, INDEX clauses, text occurrences)
- Context-aware completion (symbols, base types, module names, STATUS/ACCESS values)
- Document symbols and workspace symbol search
- Semantic token highlighting (keywords, types, identifiers, macros)
- Diagnostics from gomib parse and resolve pipeline
- Automatic workspace loading on open
- Debounced reload on save

[Unreleased]: https://github.com/golangsnmp/gomib-lsp/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/golangsnmp/gomib-lsp/releases/tag/v0.1.0
