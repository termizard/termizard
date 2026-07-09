package config

import "testing"

func TestShellBaseStripsPathAndExe(t *testing.T) {
	if got := shellBase("/usr/bin/zsh"); got != "zsh" {
		t.Fatalf("zsh = %q", got)
	}
	if got := shellBase("/opt/homebrew/bin/bash"); got != "bash" {
		t.Fatalf("bash = %q", got)
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
