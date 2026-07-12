# termizard: keep the default PowerShell prompt, but publish cwd via OSC 0/2
# so the window/tab title matches `PS C:\Users\...>` inside the terminal.

function global:prompt {
    $path = $PWD.Path
    $esc = [char]27
    $bell = [char]7
    [Console]::Write("$esc]0;$path$bell")
    "PS $($executionContext.SessionState.Path.CurrentLocation)$('>' * ($nestedPromptLevel + 1)) "
}
