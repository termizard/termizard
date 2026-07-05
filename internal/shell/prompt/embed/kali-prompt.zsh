# Shared Kali twoline prompt layout for termizard

setopt prompt_subst 2>/dev/null

prompt_termizard_kali_precmd() {
  print -Pn "\e]0;%~\a"
}

if typeset -f add-zsh-hook >/dev/null 2>&1; then
  add-zsh-hook precmd prompt_termizard_kali_precmd
else
  precmd() { print -Pn "\e]0;%~\a" }
fi

# Two-line box prompt: ┌──(user㉿host)-[path] / └─❯
# Use heavy ❯ (not thin → / ➜) — reads larger in Menlo and similar fonts.
PROMPT=$'┌──(%n㉿%m)-[%1~]\n└─%(#.%B%F{red}#%f%b.%B%F{red}❯%f%b) '
RPROMPT=''

if [[ -z "${TERMIZARD_PROMPT_INIT:-}" ]]; then
  export TERMIZARD_PROMPT_INIT=1
  print
fi
