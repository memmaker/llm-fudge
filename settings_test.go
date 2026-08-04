package main

import (
	"strings"
	"testing"

	"fudge/prompts"
)

func TestSettingsFollowPromptLanguage(t *testing.T) {
	t.Cleanup(func() {
		if err := prompts.SetLanguage("en"); err != nil {
			t.Fatal(err)
		}
	})

	for _, language := range []string{"en", "de"} {
		if err := prompts.SetLanguage(language); err != nil {
			t.Fatal(err)
		}

		settings, err := listAvailableSettings()
		if err != nil {
			t.Fatalf("%s: list settings: %v", language, err)
		}
		if len(settings) != 5 {
			t.Fatalf("%s: got %d settings, want 5: %v", language, len(settings), settings)
		}

		s := newSession()
		if err := loadSetting(s, "medieval-fantasy"); err != nil {
			t.Fatalf("%s: load setting: %v", language, err)
		}
		if strings.TrimSpace(s.settingDesc) == "" {
			t.Fatalf("%s: setting description is empty", language)
		}
		adventures, err := listAvailableAdventures(s)
		if err != nil {
			t.Fatalf("%s: list adventures: %v", language, err)
		}
		if len(adventures) != 2 {
			t.Fatalf("%s: got %d adventures, want 2: %v", language, len(adventures), adventures)
		}
		adventure, err := loadAdventure(s, adventures[0])
		if err != nil {
			t.Fatalf("%s: load adventure: %v", language, err)
		}
		if strings.TrimSpace(adventure) == "" {
			t.Fatalf("%s: adventure is empty", language)
		}
	}
}
