package prompts

import (
	"fmt"
	"os"
	"strings"

	"fudge/rules"
)

// LoadPrompt reads a system prompt and combines it with always.md
func LoadPrompt(filename string, setting *rules.SettingInfos) (string, error) {
	always, err := os.ReadFile("system-prompts/always.md")
	if err != nil {
		return "", fmt.Errorf("failed to load always.md: %w", err)
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("failed to load prompt file %q: %w", filename, err)
	}

	// Replace ESC placeholders with actual escape character
	alwaysStr := strings.ReplaceAll(string(always), "ESC", "\033")
	prompt := alwaysStr + "\n\n" + string(content)

	// Append context if provided
	if setting != nil {
		prompt += "\n## Setting Information:\n\n" + setting.ToString()
	}

	return prompt, nil
}
