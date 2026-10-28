//go:build !windows

package sshconn

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"motty/internal/localshell"
	"motty/internal/term"
)

func TestIntegrationReportsRealShellDirectoriesWithoutChangingProfiles(t *testing.T) {
	for _, name := range []string{"bash", "zsh"} {
		t.Run(name, func(t *testing.T) {
			program, err := exec.LookPath(name)
			if err != nil {
				t.Skipf("%s unavailable: %v", name, err)
			}
			home, runtimeDir := t.TempDir(), t.TempDir()
			target := filepath.Join(home, "a b%'#;$()", "caf\u00e9")
			if err := os.MkdirAll(target, 0700); err != nil {
				t.Fatal(err)
			}
			profile := "printf 'user-profile-loaded\\n'\n"
			file := ".bash_profile"
			if name == "bash" {
				profile += "PS1='user-prompt> '; PROMPT_COMMAND='printf \"user-hook\\n\"'\n"
			} else {
				file = ".zshrc"
				profile += "PS1='user-prompt> '; precmd() { printf 'user-hook\\n'; }\n"
			}
			profilePath := filepath.Join(home, file)
			if err := os.WriteFile(profilePath, []byte(profile), 0600); err != nil {
				t.Fatal(err)
			}
			shell, err := localshell.Start(localshell.Config{
				Shell: "/bin/sh", Args: []string{"-c", integrationCommand(program)}, Dir: home,
				Env:  []string{"HOME=" + home, "ZDOTDIR=" + home, "SHELL=" + program, "TMPDIR=" + runtimeDir},
				Cols: 120, Rows: 24,
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(shell.Close)
			screen := term.New(120, 24, 0)
			ready, output := 0, ""
			screen.SetPromptReady(func() { ready++ })
			await := func(want string, predicate func() bool) {
				t.Helper()
				deadline := time.Now().Add(8 * time.Second)
				for time.Now().Before(deadline) {
					data := shell.Drain(nil)
					output += string(data)
					screen.Write(data)
					if predicate() {
						return
					}
					time.Sleep(5 * time.Millisecond)
				}
				t.Fatalf("waiting for %s: %q", want, output)
			}
			await("initial ready prompt", func() bool { return ready > 0 })
			if !strings.Contains(output, "user-profile-loaded") || !strings.Contains(output, "user-hook") {
				t.Fatalf("profile/hook lost: %q", output)
			}
			before := ready
			shell.Write([]byte("cd -- '" + strings.ReplaceAll(target, "'", "'\\''") + "'\r"))
			await("cd report", func() bool {
				cwd, ok := screen.WorkingDirectory()
				return ok && cwd.Path == target && ready > before
			})
			if strings.Count(output, "user-hook\r\n") < 2 {
				t.Fatalf("hook was overwritten: %q", output)
			}
			if name == "zsh" {
				shell.Write([]byte("printf 'zdot=%s\\n' \"$ZDOTDIR\"\r"))
				await("restored ZDOTDIR", func() bool { return strings.Contains(output, "zdot="+home+"\r\n") })
			}
			shell.Write([]byte("exit 7\r"))
			select {
			case <-shell.Done():
			case <-time.After(8 * time.Second):
				t.Fatal("integrated shell did not exit")
			}
			if shell.ExitStatus() != 7 {
				t.Fatalf("exit status = %d, want 7", shell.ExitStatus())
			}
			if data, err := os.ReadFile(profilePath); err != nil || string(data) != profile {
				t.Fatalf("profile changed: %q, %v", data, err)
			}
			if files, err := os.ReadDir(runtimeDir); err != nil || len(files) != 0 {
				t.Fatalf("session files leaked: %v, %v", files, err)
			}
		})
	}
}

func TestIntegrationRejectsUnsupportedShells(t *testing.T) {
	for _, shell := range []string{"", "bash", "/bin/fish", "/bin/bash\n", "/bin/bash;touch /tmp/no"} {
		if command := integrationCommand(shell); command != "" {
			t.Errorf("unsupported shell %q enabled integration", shell)
		}
	}
}
