package prompts

import (
	"fmt"
	"os"
	"strings"
)

// DefaultLanguage is what the player reads unless they ask for another one. It
// is the language every prompt file is already written in, so it needs no
// directive of its own.
const DefaultLanguage = "English"

// languageCodes covers the ISO 639-1 forms a player is likely to reach for. A
// bare "de" dropped into the prompt would be genuinely ambiguous—it is a word in
// several languages and the name of none—so the flag resolves to a language name
// before the prompt is ever built. Anything not in here is passed through as
// written: "German", "Deutsch" and "Brazilian Portuguese" all name a language
// well enough for the model, and no list would ever be complete.
var languageCodes = map[string]string{
	"ar": "Arabic",
	"cs": "Czech",
	"da": "Danish",
	"de": "German",
	"el": "Greek",
	"en": "English",
	"es": "Spanish",
	"fi": "Finnish",
	"fr": "French",
	"he": "Hebrew",
	"hu": "Hungarian",
	"it": "Italian",
	"ja": "Japanese",
	"ko": "Korean",
	"nl": "Dutch",
	"no": "Norwegian",
	"pl": "Polish",
	"pt": "Portuguese",
	"ro": "Romanian",
	"ru": "Russian",
	"sv": "Swedish",
	"tr": "Turkish",
	"uk": "Ukrainian",
	"zh": "Chinese",
}

// NormalizeLanguage turns whatever the player asked for into the name the prompt
// says out loud. Empty means English.
func NormalizeLanguage(language string) string {
	language = strings.TrimSpace(language)
	if language == "" {
		return DefaultLanguage
	}
	if name, ok := languageCodes[strings.ToLower(language)]; ok {
		return name
	}
	// However it was capitalised, English is English: it is the one answer that
	// changes what gets built rather than only what the directive says.
	if strings.EqualFold(language, DefaultLanguage) {
		return DefaultLanguage
	}
	return language
}

// LanguageSection is the directive that splits what the player reads from what
// the engine stores. It comes back empty for English: every prompt file is
// already written in it, so there is nothing to say and no reason to spend the
// tokens saying it.
func LanguageSection(language string) (string, error) {
	if NormalizeLanguage(language) == DefaultLanguage {
		return "", nil
	}
	content, err := os.ReadFile("system-prompts/language.md")
	if err != nil {
		return "", fmt.Errorf("failed to load language.md: %w", err)
	}
	return strings.ReplaceAll(string(content), "{{LANGUAGE}}", NormalizeLanguage(language)), nil
}

// LoadPrompt assembles one system prompt: always.md, then each of the given
// prompt files in order, then the setting, then the language directive. The
// wrapper appears exactly once no matter how many files are combined—always.md
// alone is 12KB of rules reference, and loading two prompts separately and
// concatenating them would duplicate that (and the setting) in every request.
//
// The language directive goes last because it is the one rule that has to
// survive everything above it: the files it overrides are themselves written in
// English, and the setting between them is a list of English slugs.
func LoadPrompt(setting string, language string, filenames ...string) (string, error) {
	always, err := os.ReadFile("system-prompts/always.md")
	if err != nil {
		return "", fmt.Errorf("failed to load always.md: %w", err)
	}

	var b strings.Builder
	b.Write(always)

	for _, filename := range filenames {
		content, err := os.ReadFile(filename)
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

	languageSection, err := LanguageSection(language)
	if err != nil {
		return "", err
	}
	if languageSection != "" {
		b.WriteString("\n\n")
		b.WriteString(languageSection)
	}

	return b.String(), nil
}
