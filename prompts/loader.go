package prompts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Prompt files are grouped by language. English is the default; callers select
// another language once at startup and every subsequent load uses that set.

// root is where every language-specific prompt set lives. Prompts are read from disk rather than
// embedded so they can be edited between runs without a rebuild, which is most
// of what working on this project consists of — but the path is resolved once,
// at init, so a later chdir cannot move it out from under a turn in progress.
var root = absolutePromptDir()
var language = "en"

func absolutePromptDir() string {
	abs, err := filepath.Abs("prompts")
	if err != nil {
		return "prompts"
	}
	return abs
}

// SetLanguage selects the prompt directory used by subsequent loads.
func SetLanguage(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\\`) {
		return fmt.Errorf("invalid language %q", value)
	}
	info, err := os.Stat(filepath.Join(root, value))
	if err != nil {
		return fmt.Errorf("language %q is not available: %w", value, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("language %q is not a prompt directory", value)
	}
	language = value
	return nil
}

// Language reports the currently selected prompt language.
func Language() string { return language }

// Path is the full path of one prompt file, for the callers that read a prompt
// themselves rather than through Fill.
func Path(name string) string { return filepath.Join(root, language, name) }

// Fill reads one prompt file and substitutes each {{KEY}} with its value. A key
// left in the file with nothing to fill it is an error rather than a
// placeholder shipped to the model.
func Fill(name string, vars map[string]string) (string, error) {
	content, err := os.ReadFile(Path(name))
	if err != nil {
		return "", fmt.Errorf("failed to load %s prompt %q: %w", language, name, err)
	}
	return substitute(name, string(content), vars)
}

// MustFill loads a prompt that is required to construct a request. It is
// intended for static tool metadata, where a missing file is a configuration
// error that should stop startup rather than silently sending an empty field.
func MustFill(name string, vars map[string]string) string {
	text, err := Fill(name, vars)
	if err != nil {
		panic(err)
	}
	return text
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

// LoadPrompt assembles one system prompt: each of the given prompt files in
// order, then the setting. The {{KEY}} substitution runs over the assembled
// whole — a prompt names a file at authoring time and receives its contents at
// runtime, which is the only way a pointer survives a turn that has no read
// tool to follow one with.
//
// There is no wrapper file any more. The Fudge reference used to be prepended
// to every system prompt this built, including the two setup prompts that have
// no use for a ladder; it lives in dm.md now, where its only reader is.
func LoadPrompt(setting string, vars map[string]string, filenames ...string) (string, error) {
	var b strings.Builder

	for i, filename := range filenames {
		content, err := os.ReadFile(Path(filename))
		if err != nil {
			return "", fmt.Errorf("failed to load %s prompt file %q: %w", language, filename, err)
		}
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.Write(content)
	}

	// Append the setting's own prose if there is any. It is a description now,
	// not a table of slugs: nothing validates what the DM invents inside it.
	if setting != "" {
		content, err := os.ReadFile(Path("setting-information.md"))
		if err != nil {
			return "", fmt.Errorf("failed to load %s prompt file %q: %w", language, "setting-information.md", err)
		}
		b.WriteString("\n")
		b.Write(content)
	}

	allVars := make(map[string]string, len(vars)+1)
	for key, value := range vars {
		allVars[key] = value
	}
	allVars["SETTING"] = setting
	return substitute(strings.Join(filenames, "+"), b.String(), allVars)
}

// Fingerprint is a short hash of the prompt set actually on disk. Prompts are
// read at run time rather than embedded, so the binary's commit says nothing
// about them: a prompt file edited between two runs leaves both claiming the
// same revision. This is the identity a measurement is comparable under.
//
// Names are hashed alongside contents, so an added or renamed file changes the
// answer even when nothing else does.
func Fingerprint() string {
	dir := filepath.Join(root, language)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	sum := sha256.New()
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	// Sorted, because ReadDir's order is not promised across filesystems and a
	// fingerprint that depends on the machine compares nothing.
	sort.Strings(names)
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return ""
		}
		fmt.Fprintf(sum, "%s\n%d\n", name, len(body))
		sum.Write(body)
	}
	return hex.EncodeToString(sum.Sum(nil))[:12]
}
