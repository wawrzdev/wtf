_wtf_complete() {
  local cur prev
  cur=${COMP_WORDS[COMP_CWORD]}
  prev=${COMP_WORDS[COMP_CWORD-1]}
  if (( COMP_CWORD == 1 )); then
    COMPREPLY=( $(compgen -W "list topic cache init completion version $(command wtf list -a 2>/dev/null | cut -f2)" -- "$cur") )
  elif [[ ${COMP_WORDS[1]} == list ]]; then
    COMPREPLY=( $(compgen -W '-a --all --json' -- "$cur") )
  elif [[ ${COMP_WORDS[1]} == topic && $COMP_CWORD == 2 ]]; then
    COMPREPLY=( $(compgen -W 'new edit' -- "$cur") )
  elif [[ ${COMP_WORDS[1]} == cache && $COMP_CWORD == 2 ]]; then
    COMPREPLY=( $(compgen -W 'status refresh' -- "$cur") )
  elif [[ ${COMP_WORDS[1]} == cache && ${COMP_WORDS[2]} == status ]]; then
    COMPREPLY=( $(compgen -W '--json' -- "$cur") )
  elif [[ ${COMP_WORDS[1]} == init ]]; then
    COMPREPLY=( $(compgen -W 'zsh' -- "$cur") )
  elif [[ ${COMP_WORDS[1]} == completion ]]; then
    COMPREPLY=( $(compgen -W 'zsh bash fish' -- "$cur") )
  elif [[ ${COMP_WORDS[1]} == topic && ${COMP_WORDS[2]} == edit ]]; then
    COMPREPLY=( $(compgen -W "$(command wtf list -a 2>/dev/null | awk -F '\t' '$1 == "topic" {print $2}')" -- "$cur") )
  fi
}
complete -F _wtf_complete wtf
