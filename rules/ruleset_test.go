package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRulesetSupportsMissingOptionalFiles(t *testing.T) {
	path := t.TempDir()
	writeFile(t, filepath.Join(path, "attributes.rec"), "slug: strength\ndisplay_name: Strength\ndescription: Raw might\n")

	ruleset, err := LoadRuleset(path)
	if err != nil {
		t.Fatalf("LoadRuleset returned error: %v", err)
	}
	if ruleset.Slug != filepath.Base(path) {
		t.Fatalf("ruleset slug = %q", ruleset.Slug)
	}
	if len(ruleset.Attributes) != 1 {
		t.Fatalf("attributes count = %d", len(ruleset.Attributes))
	}
	if len(ruleset.Skills)+len(ruleset.Gifts)+len(ruleset.Faults)+len(ruleset.Powers)+len(ruleset.Species) != 0 {
		t.Fatalf("optional definitions should be empty")
	}
}

func TestLoadRulesetRejectsInvalidDefinitions(t *testing.T) {
	t.Run("invalid slug", func(t *testing.T) {
		path := t.TempDir()
		writeFile(t, filepath.Join(path, "attributes.rec"), "slug: Invalid Slug\ndisplay_name: Strength\ndescription: Raw might\n")
		_, err := LoadRuleset(path)
		if err == nil || !strings.Contains(err.Error(), "slug") {
			t.Fatalf("expected slug error, got: %v", err)
		}
	})

	t.Run("species scale bounds", func(t *testing.T) {
		path := t.TempDir()
		writeFile(t, filepath.Join(path, "attributes.rec"), "slug: strength\ndisplay_name: Strength\ndescription: Raw might\n")
		writeFile(t, filepath.Join(path, "species.rec"), "slug: giant\ndisplay_name: Giant\ndescription: Too big\nscale: 11\n")
		_, err := LoadRuleset(path)
		if err == nil || !strings.Contains(err.Error(), "scale") {
			t.Fatalf("expected scale error, got: %v", err)
		}
	})
}

func TestRulesetWriteFileSafetyAndSyntax(t *testing.T) {
	path := t.TempDir()
	writeFile(t, filepath.Join(path, "attributes.rec"), "slug: strength\ndisplay_name: Strength\ndescription: Raw might\n")
	ruleset, err := LoadRuleset(path)
	if err != nil {
		t.Fatalf("LoadRuleset returned error: %v", err)
	}

	if err := ruleset.WriteFile("../hack.rec", "replace", []byte("slug: bad\n")); err == nil {
		t.Fatal("expected unsafe path error")
	}
	if err := ruleset.WriteFile("unknown.rec", "replace", []byte("slug: bad\n")); err == nil {
		t.Fatal("expected unsupported file error")
	}
	if err := ruleset.WriteFile("skills.rec", "bad", []byte("slug: bad\n")); err == nil {
		t.Fatal("expected mode error")
	}
	if err := ruleset.WriteFile("skills.rec", "replace", []byte("not-a-field\n")); err == nil {
		t.Fatal("expected syntax error")
	}
	if err := ruleset.WriteFile("skills.rec", "replace", []byte("slug: athletics\ndisplay_name: Athletics\ndescription: Move quickly\n")); err != nil {
		t.Fatalf("replace write failed: %v", err)
	}
	if err := ruleset.WriteFile("skills.rec", "append", []byte("\nslug: stealth\ndisplay_name: Stealth\ndescription: Move quietly\n")); err != nil {
		t.Fatalf("append write failed: %v", err)
	}
	reloaded, err := LoadRuleset(path)
	if err != nil {
		t.Fatalf("reloading ruleset failed: %v", err)
	}
	if len(reloaded.Skills) != 2 {
		t.Fatalf("skills count = %d", len(reloaded.Skills))
	}
}

func TestNewWorldLoadsRuleset(t *testing.T) {
	path := t.TempDir()
	writeFile(t, filepath.Join(path, "attributes.rec"), "slug: strength\ndisplay_name: Strength\ndescription: Raw might\n")

	world, err := NewWorld(7, path)
	if err != nil {
		t.Fatalf("NewWorld returned error: %v", err)
	}
	if world.Ruleset == nil {
		t.Fatal("world ruleset is nil")
	}
	if world.Ruleset.Slug != filepath.Base(path) {
		t.Fatalf("world ruleset slug = %q", world.Ruleset.Slug)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
