# go-clue for Vim

Vim adapter for typed Go templates powered by `go-clue lsp`.

Classic Vim does not include a built-in LSP client, so this package registers
`go-clue lsp` with the popular `vim-lsp` plugin.

## Requirements

- Vim 8.2 or newer
- `prabirshrestha/vim-lsp`
- `go-clue` on `PATH`

Install the CLI:

```bash
go install github.com/donseba/go-clue@latest
```

## Install

With `vim-plug`:

```vim
Plug 'prabirshrestha/vim-lsp'
Plug 'donseba/go-clue', { 'rtp': 'ide/vim' }
```

From a release ZIP, copy the contents of `go-clue-vim` into:

```text
~/.vim/pack/go-clue/start/go-clue
```

On Windows:

```text
%USERPROFILE%\vimfiles\pack\go-clue\start\go-clue
```

## Quick Start

The plugin auto-registers the server on `User lsp_setup`:

```text
go-clue lsp <nearest go.mod directory>
```

Disable automatic registration:

```vim
let g:go_clue_auto_start = 0
```

Then copy the server registration from `plugin/go_clue_lsp.vim` into your own
Vim config and customize it.

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

The Vim package only registers the server. Completion, diagnostics, hover,
go-to-definition, and document symbols come from `go-clue lsp`. The server
understands `@model`, `@dot`, `@func`, range/with dot context, typed function
returns, template includes, named defines, and block calls.
