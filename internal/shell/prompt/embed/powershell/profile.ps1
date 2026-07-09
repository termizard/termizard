# termizard bundled Kali-style prompt for PowerShell (OSC window title + twoline layout).

function global:prompt {
    $path = $PWD.Path
    $homePath = $env:USERPROFILE
    $short = $path
    if ($homePath -and $path.StartsWith($homePath, [System.StringComparison]::OrdinalIgnoreCase)) {
        $short = '~' + $path.Substring($homePath.Length)
    }
    $esc = [char]27
    $bell = [char]7
    [Console]::Write("$esc]0;$short$bell")

    $user = $env:USERNAME
    $hostName = $env:COMPUTERNAME
    $isAdmin = $false
    try {
        $isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    } catch {
        $isAdmin = $false
    }

    $segment = Split-Path -Leaf $short
    if ($segment -eq '') { $segment = $short }

    Write-Host ""
    Write-Host "┌──(" -NoNewline -ForegroundColor DarkGray
    Write-Host "$user" -NoNewline -ForegroundColor Cyan
    Write-Host "㉿" -NoNewline -ForegroundColor DarkGray
    Write-Host "$hostName" -NoNewline -ForegroundColor Cyan
    Write-Host ")-[" -NoNewline -ForegroundColor DarkGray
    Write-Host "$segment" -NoNewline -ForegroundColor Blue
    Write-Host "]" -ForegroundColor DarkGray
    if ($isAdmin) {
        Write-Host "└─" -NoNewline -ForegroundColor DarkGray
        Write-Host "# " -NoNewline -ForegroundColor Red
    } else {
        Write-Host "└─" -NoNewline -ForegroundColor DarkGray
        Write-Host "$ " -NoNewline -ForegroundColor Green
    }
    return ""
}
