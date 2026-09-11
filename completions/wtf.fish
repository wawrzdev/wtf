complete -c wtf -f
function __wtf_topic_ids
  command wtf list -a 2>/dev/null | awk -F '\t' '$1 == "topic" {print $2}'
end
complete -c wtf -n '__fish_use_subcommand' -a list -d 'List commands'
complete -c wtf -n '__fish_use_subcommand' -a topic -d 'Author topics'
complete -c wtf -n '__fish_use_subcommand' -a cache -d 'Inspect package cache'
complete -c wtf -n '__fish_use_subcommand' -a init -d 'Emit shell integration'
complete -c wtf -n '__fish_use_subcommand' -a completion -d 'Emit completions'
complete -c wtf -n '__fish_use_subcommand' -a version -d 'Print version'
complete -c wtf -n '__fish_seen_subcommand_from list' -s a -l all -d 'Include unavailable annotations'
complete -c wtf -n '__fish_seen_subcommand_from list' -l json -d 'Emit JSON'
complete -c wtf -n '__fish_seen_subcommand_from topic; and not __fish_seen_subcommand_from new edit' -a 'new edit'
complete -c wtf -n '__fish_seen_subcommand_from topic; and __fish_seen_subcommand_from edit' -a '(__wtf_topic_ids)'
complete -c wtf -n '__fish_seen_subcommand_from cache; and not __fish_seen_subcommand_from status refresh' -a 'status refresh'
complete -c wtf -n '__fish_seen_subcommand_from cache; and __fish_seen_subcommand_from status' -l json -d 'Emit JSON'
complete -c wtf -n '__fish_seen_subcommand_from init' -a zsh
complete -c wtf -n '__fish_seen_subcommand_from completion' -a 'zsh bash fish'
