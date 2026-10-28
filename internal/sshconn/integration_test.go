package sshconn

import (
	"io"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestIntegrationSSHStartupAndFallback(t *testing.T) {
	for _, mode := range []string{"integrated", "probe-refused", "unsupported", "exec-refused", "custom-command"} {
		t.Run(mode, func(t *testing.T) {
			server := startServer(t, func(server *testServer) {
				server.password, server.shell = "secret", echoShell
				server.command = func(command string, channel ssh.Channel) {
					defer channel.Close()
					text := "integrated-startup"
					if command == shellIntegrationProbe {
						text = "\x1emotty-shell:/bin/bash\x1f"
						if mode == "unsupported" {
							text = "\x1emotty-shell:/bin/fish\x1f"
						}
					} else if mode == "custom-command" {
						if command != "custom shell" {
							t.Errorf("custom command replaced by %q", command)
						}
						text = "custom-startup"
					}
					io.WriteString(channel, text)
					channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
				}
				server.refuseCommand = func(command string) bool {
					return mode == "probe-refused" && command == shellIntegrationProbe ||
						mode == "exec-refused" && command != shellIntegrationProbe
				}
			})
			h := connect(t, server)
			config := ShellConfig{Cols: 80, Rows: 24, IntegrateShell: true}
			want := "integrated-startup"
			if mode == "custom-command" {
				config.Command, want = "custom shell", "custom-startup"
			} else if mode != "integrated" {
				want = "welcome"
			}
			w := openShellWith(t, h, config)
			w.until(want, func(output string) bool { return strings.Contains(output, want) })
		})
	}
}
