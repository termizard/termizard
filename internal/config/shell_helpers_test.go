package config

import (
	"strings"
	"testing"
)

func TestShellBaseStripsPathAndExe(t *testing.T) {
	if got := shellBase("/usr/bin/zsh"); got != "zsh" {
		t.Fatalf("zsh = %q", got)
	}
	if got := shellBase("/opt/homebrew/bin/bash"); got != "bash" {
		t.Fatalf("bash = %q", got)
	}
	if got := shellBase(`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`); got != "powershell" {
		t.Fatalf("windows powershell = %q", got)
	}
}

func TestHasArgAndRemoveArgs(t *testing.T) {
	args := []string{"-l", "-f", "--login"}
	if !hasArg(args, "-f") {
		t.Fatal("missing -f")
	}
	out := removeArgs(args, "-f", "-missing")
	if len(out) != 2 || out[0] != "-l" {
		t.Fatalf("removeArgs = %v", out)
	}
}

func TestWithBundledPowerShellNoProfile(t *testing.T) {
	cmd := withBundledPowerShell([]string{"pwsh.exe"}, "")
	if len(cmd) < 2 || cmd[1] != "-NoLogo" {
		t.Fatalf("cmd = %v", cmd)
	}
}

func TestWithBundledPowerShellWithProfile(t *testing.T) {
	cmd := withBundledPowerShell([]string{"powershell"}, "/tmp/profile.ps1")
	if len(cmd) < 4 || cmd[len(cmd)-1] != "/tmp/profile.ps1" {
		t.Fatalf("cmd = %v", cmd)
	}
}

func TestWithPowerShellCwdTitleInjectsFile(t *testing.T) {
	cmd := withPowerShellCwdTitle([]string{`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`})
	if len(cmd) < 6 {
		t.Fatalf("cmd too short: %v", cmd)
	}
	if cmd[1] != "-NoLogo" || cmd[2] != psNoExit {
		t.Fatalf("flags = %v", cmd)
	}
	foundFile := false
	for i, a := range cmd {
		if a == psFile && i+1 < len(cmd) && strings.HasSuffix(cmd[i+1], "title.ps1") {
			foundFile = true
		}
	}
	if !foundFile {
		t.Fatalf("expected -File title.ps1, got %v", cmd)
	}
}

func TestWithPowerShellCwdTitleSkipsExistingFile(t *testing.T) {
	in := []string{"pwsh", "-NoLogo", psNoExit, psFile, "/tmp/other.ps1"}
	got := withPowerShellCwdTitle(in)
	if len(got) != len(in) || got[4] != "/tmp/other.ps1" {
		t.Fatalf("should preserve -File: %v", got)
	}
}

func TestWithPowerShellCwdTitleIgnoresCmd(t *testing.T) {
	in := []string{"cmd.exe", "/K"}
	got := withPowerShellCwdTitle(in)
	if len(got) != len(in) {
		t.Fatalf("cmd.exe mutated: %v", got)
	}
}

func TestWithInteractiveZshStripsNoRCS(t *testing.T) {
	cmd := withInteractiveZsh([]string{"/bin/zsh", "-f", "-l"})
	if cmd[1] != "-i" {
		t.Fatalf("cmd = %v", cmd)
	}
	for _, a := range cmd {
		if a == "-f" {
			t.Fatalf("should strip -f: %v", cmd)
		}
	}
}
