package prompts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// A prompt set is monolingual: the language the player reads is the language
// these files are written in, and there is no directive asking for one. Playing
// in another language means translating the set. An instruction to switch can
// be disobeyed, and every language slip observed in play was meta text escaping
// into the player's channel rather than a translation failure. See docs/adr/007.

// dir is where every prompt file lives. Prompts are read from disk rather than
// embedded so they can be edited between runs without a rebuild, which is most
// of what working on this project consists of — but the path is resolved once,
// at init, so a later chdir cannot move it out from under a turn in progress.
var dir = absolutePromptDir()

func absolutePromptDir() string {
	abs, err := filepath.Abs("system-prompts")
	if err != nil {
		return "system-prompts"
	}
	return abs
}

// Path is the full path of one prompt file, for the callers that read a prompt
// themselves rather than through Fill.
func Path(name string) string { return filepath.Join(dir, name) }

// Fill reads one prompt file and substitutes each {{KEY}} with its value. A key
// left in the file with nothing to fill it is an error rather than a
// placeholder shipped to the model.
func Fill(name string, vars map[string]string) (string, error) {
	content, err := os.ReadFile(Path(name))
	if err != nil {
		return "", fmt.Errorf("failed to load prompt %q: %w", name, err)
	}
	return substitute(name, string(content), vars)
}

func substitute(name, text string, vars map[string]string) (string, error) {
	for key, value := range vars {
		text = strings.ReplaceAll(text, "{{"+key+"}}", value)
	}
	if i := strings.Index(text, "{{"); i >= 0 {
		end := strings.Index(text[i:], "}}")
		if end < 0 {
			end = len(text[i:]) - 1
		}
		return "", fmt.Errorf("prompt %q has an unfilled placeholder %s", name, text[i:i+end+2])
	}
	return strings.TrimSpace(text), nil
}

// LoadPrompt assembles one system prompt: always.md, then each of the given
// prompt files in order, then the setting. The wrapper appears exactly once no
// matter how many files are combined, and the {{KEY}} substitution runs over
// the assembled whole — a prompt names a file at authoring time and receives its
// contents at runtime, which is the only way a pointer survives a turn that has
// no read tool to follow one with.
func LoadPrompt(setting string, vars map[string]string, filenames ...string) (string, error) {
	always, err := os.ReadFile(Path("always.md"))
	if err != nil {
		return "", fmt.Errorf("failed to load always.md: %w", err)
	}

	var b strings.Builder
	b.Write(always)

	for _, filename := range filenames {
		content, err := os.ReadFile(Path(filename))
		if err != nil {
			return "", fmt.Errorf("failed to load prompt file %q: %w", filename, err)
		}
		b.WriteString("\n\n")
		b.Write(content)
	}

	// Append the setting's own prose if there is any. It is a description now,
	// not a table of slugs: nothing validates what the DM invents inside it.
	if setting != "" {
		b.WriteString("\n## Setting Information:\n\n")
		b.WriteString(setting)
	}

	return substitute(strings.Join(filenames, "+"), b.String(), vars)
}
