package workinsight

import "strings"

// classifyDeveloperTool maps common client user-agent fingerprints to stable display names.
func classifyDeveloperTool(userAgent string) string {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	for _, item := range []struct {
		name string
		keys []string
	}{
		{"Cursor", []string{"cursor"}},
		{"Claude Code", []string{"claude-cli", "claude_cli", "claude-code", "claude_code", "claude code", "claude-code-vscode"}},
		{"Claude Desktop", []string{"claude desktop"}},
		{"Codex Desktop", []string{"codex desktop", "codex-desktop", "chatgpt desktop", "chatgpt-desktop"}},
		{"Codex CLI", []string{"codex-cli", "codex_cli", "codex_cli_rs", "openai-codex", "openai_codex"}},
		{"OpenCode", []string{"opencode", "open-code"}},
		{"OpenClaw", []string{"openclaw", "open-claw", "open_claw"}},
		{"Hermes", []string{"hermes-agent", "hermes_agent", "hermes cli", "hermes-cli"}},
		{"Pi Agent", []string{"pi-agent", "pi_agent", "pi agent", "pi-coding-agent", "pi_coding_agent"}},
		{"WorkBuddy", []string{"workbuddy", "work-buddy", "work_buddy"}},
		{"Windsurf", []string{"windsurf"}},
		{"GitHub Copilot", []string{"github-copilot", "copilot-chat", "copilot/"}},
		{"Cline", []string{"cline"}},
		{"Roo Code", []string{"roo-code", "roo_code"}},
		{"Kilo Code", []string{"kilo-code", "kilocode"}},
		{"Continue", []string{"continue.dev", "continue/"}},
		{"Codeium", []string{"codeium"}},
		{"Tabnine", []string{"tabnine"}},
		{"Aider", []string{"aider"}},
		{"Gemini CLI", []string{"gemini-cli", "gemini_cli"}},
		{"JetBrains AI", []string{"jetbrains", "intellij", "pycharm"}},
		{"Zed", []string{"zed/", "zed-editor"}},
		{"Neovim", []string{"neovim", "nvim"}},
		{"VS Code", []string{"visual studio code", "vscode", "code/"}},
	} {
		for _, key := range item.keys {
			if strings.Contains(ua, key) {
				return item.name
			}
		}
	}
	if ua == "" {
		return "未知客户端"
	}
	return "其他客户端"
}
