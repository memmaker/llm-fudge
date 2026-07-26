package main

import (
	"strings"
	"testing"

	"fudge/prompts"
	"fudge/rules"
)

// The DM prompt is assembled from two files when image generation is on. Each
// used to be wrapped in always.md and the setting separately, duplicating ~3k
// tokens of rules reference in every request.
func TestDMPromptWrapsOnce(t *testing.T) {
	w := &rules.World{Setting: &rules.SettingInfos{Slug: "deadlands", Path: "settings/deadlands"}}

	defer func(prev bool) { imgGen = prev }(imgGen)
	imgGen = true

	prompt, err := loadDMPrompt(w)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"# Output Rules", "# Fudge Rules", "## Setting Information:"} {
		if n := strings.Count(prompt, marker); n != 1 {
			t.Errorf("%q appears %d times, want 1", marker, n)
		}
	}
	if !strings.Contains(prompt, "# Image Generation") {
		t.Error("img-style.md missing from prompt")
	}
}

// The language directive is one section however many files the prompt is
// assembled from, and it is the last word in it: everything above it is written
// in English, so a rule that arrives first is a rule the rest of the prompt
// spends 12KB contradicting by example.
func TestDMPromptCarriesLanguageDirective(t *testing.T) {
	w := &rules.World{Setting: &rules.SettingInfos{Slug: "deadlands", Path: "settings/deadlands"}}

	defer func(prevImg bool, prevLang string) { imgGen, language = prevImg, prevLang }(imgGen, language)
	imgGen = true
	language = "German"

	prompt, err := loadDMPrompt(w)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(prompt, "# Language"); n != 1 {
		t.Errorf("language section appears %d times, want 1", n)
	}
	if strings.Contains(prompt, "{{LANGUAGE}}") {
		t.Error("language placeholder left unsubstituted")
	}
	if !strings.Contains(prompt, "The player reads German") {
		t.Error("language section does not name the language")
	}
	if !strings.HasSuffix(strings.TrimSpace(prompt), "never rename it in the record.") {
		t.Error("language section is not the last thing in the prompt")
	}
}

// English is the language every prompt file is already written in, so the
// default game pays nothing for the feature—not a section, not a token.
func TestEnglishPromptCarriesNoLanguageDirective(t *testing.T) {
	w := &rules.World{Setting: &rules.SettingInfos{Slug: "deadlands", Path: "settings/deadlands"}}

	defer func(prev string) { language = prev }(language)

	language = "English"
	english, err := loadDMPrompt(w)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(english, "# Language") {
		t.Error("English prompt carries a language section")
	}

	// However the flag was spelled, and whether or not it was given at all.
	for _, spelling := range []string{"", "en", "  ", "english"} {
		language = prompts.NormalizeLanguage(spelling)
		same, err := loadDMPrompt(w)
		if err != nil {
			t.Fatal(err)
		}
		if same != english {
			t.Errorf("-lang %q built a different prompt than -lang English", spelling)
		}
	}
}

func TestNormalizeLanguage(t *testing.T) {
	for in, want := range map[string]string{
		"":                    "English",
		"  ":                  "English",
		"de":                  "German",
		"DE":                  "German",
		" fr ":                "French",
		"Deutsch":             "Deutsch",
		"Brazilian Portugese": "Brazilian Portugese",
	} {
		if got := prompts.NormalizeLanguage(in); got != want {
			t.Errorf("NormalizeLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}

// The prompt says it once, in a section the model read before the turn started.
// The tool description says it in the moment the field is being written, which
// is the moment it is forgotten.
func TestNarrationToolNamesTheLanguage(t *testing.T) {
	defer func(prev string) { language = prev }(language)

	language = "German"
	prose := narrationTool().Function.Parameters.(map[string]any)["properties"].(map[string]any)["prose"].(map[string]any)["description"].(string)
	if !strings.Contains(prose, "write it in German") {
		t.Errorf("prose description does not name the language: %q", prose)
	}

	language = "English"
	prose = narrationTool().Function.Parameters.(map[string]any)["properties"].(map[string]any)["prose"].(map[string]any)["description"].(string)
	if strings.Contains(prose, "LANGUAGE:") {
		t.Errorf("English prose description carries a language note: %q", prose)
	}
}
