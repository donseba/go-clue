if vim.g.go_clue_auto_start == false then
  return
end

require("go-clue").setup()
