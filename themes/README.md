# termizard themes

Optional Oh My Zsh themes — **not** used by termizard automatically.
Copy manually if you want the same Kali-style prompt in iTerm, Terminal.app, etc.

## termizard-kali

Kali twoline layout:

```
┌──(user㉿host)-[~]
└─$
```

### Install (Oh My Zsh)

```bash
cp themes/termizard-kali.zsh-theme "${ZSH_CUSTOM:-$HOME/.oh-my-zsh/custom}/themes/"
```

In `~/.zshrc` (before `source $ZSH/oh-my-zsh.sh`):

```zsh
ZSH_THEME="termizard-kali"
```

termizard itself uses its own bundled prompt when `no_oh_my_zsh = true` — it does not
touch your `~/.zshrc` or oh-my-zsh setup.
