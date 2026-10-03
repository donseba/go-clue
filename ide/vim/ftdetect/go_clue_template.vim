augroup go_clue_template_filetypes
  autocmd!
  autocmd BufRead,BufNewFile *.gohtml setfiletype gohtml
  autocmd BufRead,BufNewFile *.tmpl setfiletype gotmpl
augroup END
