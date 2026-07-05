# termizard-kali — Kali twoline prompt for Oh My Zsh (optional, manual install)
# See themes/README.md

setopt prompt_subst

prompt_termizard_kali_precmd() {
  print -Pn "\e]0;%~\a"
}
add-zsh-hook precmd prompt_termizard_kali_precmd

PROMPT=$'┌──(%n㉿%m)-[%1~]\n└─%(#.%F{red}#%f.%F{red}❯%f) '
RPROMPT=''

if [[ -z "${TERMIZARD_PROMPT_INIT:-}" ]]; then
  export TERMIZARD_PROMPT_INIT=1
  print
fi
