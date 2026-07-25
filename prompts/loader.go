package prompts

import (
	"fmt"
	"os"
	"strings"

	"fudge/rules"
)

// LoadPrompt assembles one system prompt: always.md, then each of the given
// prompt files in order, then the setting. The wrapper appears exactly once no
// matter how many files are combined—always.md alone is 12KB of rules reference,
// and loading two prompts separately and concatenating them would duplicate that
// (and the setting) in every request.
func LoadPrompt(setting *rules.SettingInfos, filenames ...string) (string, error) {
	always, err := os.ReadFile("system-prompts/always.md")
	if err != nil {
		return "", fmt.Errorf("failed to load always.md: %w", err)
	}

	// Replace ESC placeholders with actual escape character
	var b strings.Builder
	b.WriteString(strings.ReplaceAll(string(always), "ESC", "\033"))

	for _, filename := range filenames {
		content, err := os.ReadFile(filename)
		if err != nil {
			return "", fmt.Errorf("failed to load prompt file %q: %w", filename, err)
		}
		b.WriteString("\n\n")
		b.Write(content)
	}

	// Append context if provided
	if setting != nil {
		b.WriteString("\n## Setting Information:\n\n")
		b.WriteString(setting.ToString())
	}

	return b.String(), nil
}
