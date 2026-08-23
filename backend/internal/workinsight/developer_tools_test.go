package workinsight

import "testing"

func TestClassifyDeveloperTool(t *testing.T) {
	tests := map[string]string{
		"cursor/1.2.3":                         "Cursor",
		"claude-code/2.1.7":                    "Claude Code",
		"claude-cli/2.1.241 (external, cli)":  "Claude Code",
		"OpenAI-Codex-CLI/0.1":                 "Codex CLI",
		"codex_cli_rs/0.40.0":                  "Codex CLI",
		"ChatGPT Desktop/1.0":                  "Codex Desktop",
		"opencode/1.0":                         "OpenCode",
		"openclaw/2026.1":                      "OpenClaw",
		"hermes-agent/0.4":                     "Hermes",
		"pi-agent/1.0":                         "Pi Agent",
		"workbuddy-cli/1.0":                    "WorkBuddy",
		"Visual Studio Code/1.90 continue.dev": "Continue",
		"Mozilla/5.0 (VSCode)":                 "VS Code",
		"":                                     "未知客户端",
	}
	for userAgent, want := range tests {
		if got := classifyDeveloperTool(userAgent); got != want {
			t.Errorf("classifyDeveloperTool(%q) = %q, want %q", userAgent, got, want)
		}
	}
}
