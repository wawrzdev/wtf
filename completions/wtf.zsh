#compdef wtf
_wtf() {
  local context state line
  typeset -A opt_args
  _arguments -C \
    '1:command:->command' \
    '*::argument:->arguments'
  case $state in
    command)
      _values 'command' \
        'list[list commands]' 'topic[author topics]' 'cache[inspect package cache]' \
        'init[emit shell integration]' 'completion[emit completions]' 'version[print version]' \
        ${(f)"$(command wtf list -a 2>/dev/null | cut -f2)"}
      ;;
    arguments)
      case $words[2] in
        list) _arguments '(-a --all)'{-a,--all}'[include unavailable annotations]' '--json[emit JSON]' ;;
        topic)
          if (( CURRENT == 3 )); then _values 'topic command' 'new[create topic]' 'edit[edit topic]'
          elif [[ $words[3] == edit ]]; then _values 'topic' ${(f)"$(command wtf list -a 2>/dev/null | awk -F '\t' '$1 == "topic" {print $2}')"}
          else _message 'topic title'; fi ;;
        cache)
          if (( CURRENT == 3 )); then _values 'cache command' 'status[show cache status]' 'refresh[refresh cache]'
          elif [[ $words[3] == status ]]; then _arguments '--json[emit JSON]'; fi ;;
        init) _values 'shell' zsh ;;
        completion) _values 'shell' zsh bash fish ;;
        *) _values 'command' ${(f)"$(command wtf list -a 2>/dev/null | cut -f2)"} ;;
      esac
      ;;
  esac
}
_wtf "$@"
