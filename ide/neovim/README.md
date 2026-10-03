# go-clue for Neovim

Neovim adapter for typed Go templates powered by `go-clue lsp`.

The plugin uses Neovim's built-in LSP client. It does not implement completion,
diagnostics, hover, navigation, or semantic tokens itself; all editor
intelligence comes from `go-clue lsp`.

## Requirements

- Neovim 0.10 or newer
- `go-clue` on `PATH`

Install the CLI:

```bash
go install github.com/donseba/go-clue@latest
```

## Install

With `lazy.nvim`:

```lua
{
  "donseba/go-clue",
  dir = "ide/neovim",
  ft = { "gohtml", "gotmpl", "html" },
  config = function()
    require("go-clue").setup()
  end,
}
```

From a release ZIP, copy the contents of `go-clue-neovim` into a Neovim package
directory such as:

```text
~/.local/share/nvim/site/pack/go-clue/start/go-clue
```

On Windows:

```text
%LOCALAPPDATA%\nvim-data\site\pack\go-clue\start\go-clue
```

## Configure

Default setup:

```lua
require("go-clue").setup()
```

Custom command or filetypes:

```lua
require("go-clue").setup({
  cmd = { "go-clue", "lsp" },
  filetypes = { "gohtml", "gotmpl", "html" },
  autostart = true,
})
```

The plugin finds the nearest `go.mod` and starts the server with that directory
as the root:

```text
go-clue lsp /path/to/module
```

Disable automatic startup:

```lua
vim.g.go_clue_auto_start = false
```

Then call:

```lua
require("go-clue").start()
```

Disable go-clue for one project with `.go-clue/config.json`:

```json
{
  "enabled": false
}
```

## Template Contracts

Template contracts use `@model`:

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

## LSP Features

The Neovim package only starts the server. Completion, diagnostics, hover,
go-to-definition, semantic tokens, and document symbols come from `go-clue lsp`.
The server understands `@model`, `@dot`, `@func`, range/with dot context, typed
function returns, template includes, named defines, and block calls.
