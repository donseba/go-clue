# go-clue for GoLand

GoLand integration for typed Go templates powered by `go-clue lsp`.

The plugin starts the shared language server for `.gohtml`, `.tmpl`, and `.html`
files, then adds a few GoLand-native conveniences for installation, status,
hover, and navigation.

## Requirements

Install `go-clue` on your PATH:

```bash
go install github.com/donseba/go-clue@latest
```

If `go-clue` is missing, the plugin can offer to install it with the same command.

On Windows, the plugin starts the long-lived LSP from a temporary copy of
`go-clue.exe`. That means `go install github.com/donseba/go-clue@latest` can
replace the installed binary while GoLand is open. Restart the LSP/editor to use
the newly installed version.
`Tools > Show go-clue Status` shows both the installed CLI version and the active
LSP copy version, the resolved executable, and the selected Go module root.

The plugin uses the nearest `go.mod` above each opened template, including
nested modules and projects opened from a parent folder. Each module gets its
own language server and index.

On macOS, GoLand launched from Finder may have a different `PATH` from your
terminal. The plugin checks `PATH`, `GOBIN`, the configured `GOPATH`, and the
default `~/go/bin` directory for the global executable. It also puts the
resolved Go toolchain on the language server's `PATH` so it can load packages.

When upgrading from go-doc, uninstall the old go-doc editor plugin before
installing go-clue to keep one template language server active.

## Quick Start

Install the plugin ZIP through:

```text
Settings > Plugins > Install Plugin from Disk...
```

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
highlighting.

Writing `.go-clue/index.json` is optional. Use it only when you want a generated
artifact for CI, debugging, or other tools.

Disable go-clue for one project with `.go-clue/config.json`:

```json
{
  "enabled": false
}
```

## Editor Features

- completions for typed-root types, typed root names, fields, methods, functions, and
  range/with dot contexts
- diagnostics for unknown typed roots, unknown fields, unknown typed root types,
  invalid `range` sources, bad function calls, and wrong template include data
- hover and go-to-definition for typed root types, fields, methods, functions, and
  child templates
- document symbols for declared typed roots
- semantic highlighting for typed root types, typed root names, functions, fields, and
  methods

## Actions

The plugin adds:

```text
Tools > Rebuild go-clue Index
Tools > Show go-clue Index Status
Tools > go-clue Auto Index
```

Auto-indexing watches `.go`, `.gohtml`, `.tmpl`, and `.html` files inside the
project and debounces rebuilds. It is disabled by default because the language
server indexes in memory. Enable it with the action above or with
`.go-clue/config.json`:

```json
{
  "writeIndex": true
}
```

## Build

From the repository root:

```bash
task build:goland
```

The plugin ZIP is written below:

```text
ide/goland/build/distributions
```
