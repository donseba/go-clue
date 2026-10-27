# go-clue for VS Code

VS Code integration for typed Go templates powered by `go-clue lsp`.

## Features

- completions for typed-root Go type names
- completions for typed-root names, custom functions, and dot context
  inside `range` and `with` blocks
- completions for exported fields and methods
- diagnostics for unknown typed roots, fields, invalid range sources, bad
  function arguments, unsupported function returns, and wrong template include
  data
- hover and go to definition for contract types, fields, methods, functions, and
  child templates
- rename of a typed root across a template
- document symbols for the template's typed roots and functions
- quick fixes: add a missing field, create a missing model struct, and move
  annotations that sit before a `define` inside it
- semantic highlighting for typed root types, typed root names, functions, fields, and
  methods
- optional debounced index rebuilds

## Requirements

The extension needs VS Code 1.90 or newer. Install `go-clue-vscode-*.vsix` from
the release assets with `Extensions: Install from VSIX...`, or with:

```bash
code --install-extension go-clue-vscode-*.vsix
```

Install the `go-clue` CLI and make sure it is available on `PATH`:

```bash
go install github.com/donseba/go-clue@latest
```

If the CLI is missing when the extension starts, VS Code asks before running
that install command for you.

On Windows, the extension starts the long-lived LSP from a temporary copy of
`go-clue.exe`. That means `go install github.com/donseba/go-clue@latest` can
replace the installed binary while VS Code is open. Restart the LSP to use the
newly installed version.
`go-clue: Show Index Status` shows both the installed CLI version and the active
LSP copy version.

## Quick Start

Add a template contract:

```gotemplate
{{/*
@model Page github.com/example/app.Page
*/}}
{{ Page.Title }}
```

`@model Page ...` is the editor-side entrance of the contract. Runtime code
must still register a real `Page` template accessor before parsing, usually
with go-clue's optional renderer. For plain `tmpl.Execute(w, page)` templates,
use `@dot` and `{{ .Title }}` instead.

No `.go-clue` folder is required. The language server finds `go.mod` and builds
an in-memory index for completion, diagnostics, hover, navigation, and semantic
highlighting. The workspace folder itself need not be a Go module: in a parent
folder of several modules, every template uses the nearest `go.mod` above it.

Writing `.go-clue/index.json` is optional. Use it only when you want a generated
artifact for CI, debugging, or other tools.

## Commands

- `go-clue: Rebuild Index`
- `go-clue: Show Index Status`
- `go-clue: Toggle Enabled`
- `go-clue: Toggle Auto Index`
- `go-clue: Restart LSP`

## Settings

- `go-clue.enabled`
- `go-clue.autoIndex`
- `go-clue.debounceMilliseconds`

`go-clue.enabled` is enabled by default. Disable it globally in VS Code settings,
or opt out per project with `.go-clue/config.json`:

```json
{
  "enabled": false
}
```

`go-clue.autoIndex` is disabled by default. You can also opt in per project with
`.go-clue/config.json`:

```json
{
  "writeIndex": true
}
```

Like the CLI, the extension reads `enabled` and `writeIndex` from the older
`.go-doc/config.json` when `.go-clue/config.json` does not exist.
