# termizard bundled zsh — Kali twoline prompt (no oh-my-zsh)

setopt autocd interactive_comments share_history
HISTFILE=${HOME}/.termizard_history
HISTSIZE=10000
SAVEHIST=10000

TERMIZARD_PROMPT_DIR=${ZDOTDIR:-$HOME}
source "${TERMIZARD_PROMPT_DIR}/kali-prompt.zsh"
