package main

import (
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/Panonim/search-in-terminal/internal/backend"
)

var completionData = map[string]string{
	"Commands":    "search open suggest backends config plugin completion update cache doctor version help",
	"Backends":    strings.Join(backend.Order, " "),
	"SearchFlags": "-b --backend -n --limit -p --page --json",
	"ConfigCmds":  "show path init edit keys get set",
	"PluginCmds":  "add list remove test",
	"Shells":      "bash zsh fish",
}

func cmdCompletion(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: sit completion bash|zsh|fish")
	}
	src, ok := completionScripts[args[0]]
	if !ok {
		return fmt.Errorf("no completion for %q, have: %s", args[0], completionData["Shells"])
	}
	return template.Must(template.New(args[0]).Parse(src)).Execute(os.Stdout, completionData)
}

var completionScripts = map[string]string{
	"bash": `# sit completion for bash: add  source <(sit completion bash)  to ~/.bashrc
_sit() {
  local cur="${COMP_WORDS[COMP_CWORD]}" prev="${COMP_WORDS[COMP_CWORD-1]}"
  if [[ $COMP_CWORD -eq 1 ]]; then
    COMPREPLY=($(compgen -W "{{.Commands}}" -- "$cur")); return
  fi
  case "${COMP_WORDS[1]}" in
    search)
      if [[ $prev == -b || $prev == --backend ]]; then
        COMPREPLY=($(compgen -W "{{.Backends}}" -- "$cur"))
      elif [[ $cur == -* ]]; then
        COMPREPLY=($(compgen -W "{{.SearchFlags}}" -- "$cur"))
      fi ;;
    config)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=($(compgen -W "{{.ConfigCmds}}" -- "$cur"))
      elif [[ $COMP_CWORD -eq 3 && ($prev == get || $prev == set) ]]; then
        COMPREPLY=($(compgen -W "$(sit config keys 2>/dev/null)" -- "$cur"))
      fi ;;
    plugin)
      if [[ $COMP_CWORD -eq 2 ]]; then
        COMPREPLY=($(compgen -W "{{.PluginCmds}}" -- "$cur"))
      elif [[ $COMP_CWORD -eq 3 ]]; then
        case "$prev" in
          remove|rm) COMPREPLY=($(compgen -W "$(sit plugin list --names 2>/dev/null)" -- "$cur")) ;;
          add) COMPREPLY=($(compgen -f -- "$cur")) ;;
          test) COMPREPLY=($(compgen -W "$(sit plugin list --names 2>/dev/null)" -- "$cur") $(compgen -f -- "$cur")) ;;
        esac
      fi ;;
    completion) [[ $COMP_CWORD -eq 2 ]] && COMPREPLY=($(compgen -W "{{.Shells}}" -- "$cur")) ;;
    update) COMPREPLY=($(compgen -W "--check" -- "$cur")) ;;
    cache) [[ $COMP_CWORD -eq 2 ]] && COMPREPLY=($(compgen -W "info clear" -- "$cur")) ;;
  esac
}
complete -F _sit sit
`,
	"zsh": `# sit completion for zsh: add  source <(sit completion zsh)  to ~/.zshrc after compinit
_sit() {
  if (( CURRENT == 2 )); then
    compadd -- {{.Commands}}; return
  fi
  case ${words[2]} in
    search)
      if [[ ${words[CURRENT-1]} == (-b|--backend) ]]; then
        compadd -- {{.Backends}}
      elif [[ $PREFIX == -* ]]; then
        compadd -- {{.SearchFlags}}
      fi ;;
    config)
      if (( CURRENT == 3 )); then
        compadd -- {{.ConfigCmds}}
      elif (( CURRENT == 4 )) && [[ ${words[3]} == (get|set) ]]; then
        compadd -- ${(f)"$(sit config keys 2>/dev/null)"}
      fi ;;
    plugin)
      if (( CURRENT == 3 )); then
        compadd -- {{.PluginCmds}}
      elif (( CURRENT == 4 )); then
        case ${words[3]} in
          remove|rm) compadd -- ${(f)"$(sit plugin list --names 2>/dev/null)"} ;;
          add) _files -g '*.star' ;;
          test) compadd -- ${(f)"$(sit plugin list --names 2>/dev/null)"}; _files -g '*.star' ;;
        esac
      fi ;;
    completion) (( CURRENT == 3 )) && compadd -- {{.Shells}} ;;
    update) compadd -- --check ;;
    cache) (( CURRENT == 3 )) && compadd -- info clear ;;
  esac
}
compdef _sit sit
`,
	"fish": `# sit completion for fish: sit completion fish > ~/.config/fish/completions/sit.fish
complete -c sit -f
complete -c sit -n __fish_use_subcommand -a "{{.Commands}}"
complete -c sit -n '__fish_seen_subcommand_from search' -s b -l backend -x -a "{{.Backends}}"
complete -c sit -n '__fish_seen_subcommand_from search' -s n -l limit -x
complete -c sit -n '__fish_seen_subcommand_from search' -s p -l page -x
complete -c sit -n '__fish_seen_subcommand_from search' -l json
complete -c sit -n '__fish_seen_subcommand_from config; and not __fish_seen_subcommand_from {{.ConfigCmds}}' -a "{{.ConfigCmds}}"
complete -c sit -n '__fish_seen_subcommand_from config; and __fish_seen_subcommand_from get set' -a "(sit config keys 2>/dev/null)"
complete -c sit -n '__fish_seen_subcommand_from plugin; and not __fish_seen_subcommand_from {{.PluginCmds}} rm' -a "{{.PluginCmds}}"
complete -c sit -n '__fish_seen_subcommand_from plugin; and __fish_seen_subcommand_from remove rm test' -a "(sit plugin list --names 2>/dev/null)"
complete -c sit -n '__fish_seen_subcommand_from plugin; and __fish_seen_subcommand_from add test' -F
complete -c sit -n '__fish_seen_subcommand_from completion' -a "{{.Shells}}"
complete -c sit -n '__fish_seen_subcommand_from update' -l check
complete -c sit -n '__fish_seen_subcommand_from cache' -a "info clear"
`,
}
