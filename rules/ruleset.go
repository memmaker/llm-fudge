package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"fudge/recfile"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

type Attribute struct {
	Slug        string
	DisplayName string
	Description string
}

type Skill struct {
	Slug        string
	DisplayName string
	Description string
}

type Species struct {
	Slug        string
	DisplayName string
	Description string
	Scale       int
}

type Ruleset struct {
	Slug       string
	Frozen     bool
	Path       string
	Attributes map[string]*Attribute
	Skills     map[string]*Skill
	Species    map[string]*Species
	Gifts      map[string]*Gift
	Faults     map[string]*Fault
	Powers     map[string]*Power
}

type Gift struct {
	Slug        string
	DisplayName string
	Description string
}

type Fault struct {
	Slug        string
	DisplayName string
	Description string
}

type Power struct {
	Slug        string
	DisplayName string
	Description string
}

const (
	attributesFile = "attributes.rec"
	skillsFile     = "skills.rec"
	speciesFile    = "species.rec"
	giftsFile      = "gifts.rec"
	faultsFile     = "faults.rec"
	powersFile     = "powers.rec"
)

var validFiles = map[string]bool{
	attributesFile: true,
	skillsFile:     true,
	speciesFile:    true,
	giftsFile:      true,
	faultsFile:     true,
	powersFile:     true,
}

func LoadSetting(path string) (*Ruleset, error) {
	r := &Ruleset{
		Slug:       filepath.Base(path),
		Path:       path,
		Attributes: make(map[string]*Attribute),
		Skills:     make(map[string]*Skill),
		Species:    make(map[string]*Species),
		Gifts:      make(map[string]*Gift),
		Faults:     make(map[string]*Fault),
		Powers:     make(map[string]*Power),
	}

	if err := loadAttributes(r, filepath.Join(path, attributesFile)); err != nil {
		return nil, err
	}
	if err := loadSkills(r, filepath.Join(path, skillsFile)); err != nil {
		return nil, err
	}
	if err := loadSpecies(r, filepath.Join(path, speciesFile)); err != nil {
		return nil, err
	}
	if err := loadGifts(r, filepath.Join(path, giftsFile)); err != nil {
		return nil, err
	}
	if err := loadFaults(r, filepath.Join(path, faultsFile)); err != nil {
		return nil, err
	}
	if err := loadPowers(r, filepath.Join(path, powersFile)); err != nil {
		return nil, err
	}

	return r, nil
}

func loadAttributes(r *Ruleset, path string) error {
	return loadDefinitions(path, "attribute", func(data recfile.DataMap) error {
		slug := strings.ToLower(data["slug"])
		r.Attributes[slug] = &Attribute{
			Slug:        slug,
			DisplayName: data["display_name"],
			Description: data["description"],
		}
		return nil
	})
}

func loadSkills(r *Ruleset, path string) error {
	return loadDefinitions(path, "skill", func(data recfile.DataMap) error {
		slug := strings.ToLower(data["slug"])
		r.Skills[slug] = &Skill{
			Slug:        slug,
			DisplayName: data["display_name"],
			Description: data["description"],
		}
		return nil
	})
}

func loadSpecies(r *Ruleset, path string) error {
	return loadDefinitions(path, "species", func(data recfile.DataMap) error {
		slug := strings.ToLower(data["slug"])
		scale := data.GetIntOrDefault("scale", 0)
		if scale < -10 || scale > 10 {
			return fmt.Errorf("species %s scale must be between -10 and 10, got %d", slug, scale)
		}
		r.Species[slug] = &Species{
			Slug:        slug,
			DisplayName: data["display_name"],
			Description: data["description"],
			Scale:       scale,
		}
		return nil
	})
}

func loadGifts(r *Ruleset, path string) error {
	return loadDefinitions(path, "gift", func(data recfile.DataMap) error {
		slug := strings.ToLower(data["slug"])
		r.Gifts[slug] = &Gift{
			Slug:        slug,
			DisplayName: data["display_name"],
			Description: data["description"],
		}
		return nil
	})
}

func loadFaults(r *Ruleset, path string) error {
	return loadDefinitions(path, "fault", func(data recfile.DataMap) error {
		slug := strings.ToLower(data["slug"])
		r.Faults[slug] = &Fault{
			Slug:        slug,
			DisplayName: data["display_name"],
			Description: data["description"],
		}
		return nil
	})
}

func loadPowers(r *Ruleset, path string) error {
	return loadDefinitions(path, "power", func(data recfile.DataMap) error {
		slug := strings.ToLower(data["slug"])
		r.Powers[slug] = &Power{
			Slug:        slug,
			DisplayName: data["display_name"],
			Description: data["description"],
		}
		return nil
	})
}

func loadDefinitions(path string, typeName string, populate func(recfile.DataMap) error) error {
	records, err := readRecFile(path)
	if err != nil {
		return err
	}
	for _, record := range records {
		data := record.ToMap("|")
		slug := strings.ToLower(data["slug"])
		if slug == "" {
			return fmt.Errorf("%s missing slug in %s", typeName, path)
		}
		if !slugPattern.MatchString(slug) {
			return fmt.Errorf("invalid %s slug: %s", typeName, data["slug"])
		}
		if err := populate(data); err != nil {
			return err
		}
	}
	return nil
}

func readRecFile(path string) ([]recfile.Record, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []recfile.Record{}, nil
		}
		return nil, err
	}
	defer file.Close()
	return recfile.Read(file), nil
}

func validateRecSyntax(content []byte) error {
	text := string(content)
	if text == "" {
		return nil
	}

	lines := strings.Split(text, "\n")
	fieldPattern := regexp.MustCompile(`^[a-zA-Z%][a-zA-Z0-9_]*:`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "+") {
			continue
		}
		if !fieldPattern.MatchString(line) {
			return fmt.Errorf("invalid rec file syntax: %q (lines must start with field name followed by colon)", line)
		}
	}
	return nil
}

func (r *Ruleset) WriteFile(filename string, mode string, content []byte) error {
	if r.Frozen {
		return fmt.Errorf("setting is frozen")
	}

	if !validFiles[filename] {
		return fmt.Errorf("invalid setting file: %s", filename)
	}

	if mode != "append" && mode != "replace" {
		return fmt.Errorf("invalid write mode: %s", mode)
	}

	filePath := filepath.Join(r.Path, filename)

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return err
	}
	absRulesetPath, err := filepath.Abs(r.Path)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(absPath, absRulesetPath+string(filepath.Separator)) && absPath != filepath.Join(absRulesetPath, filename) {
		return fmt.Errorf("unsafe path: %s", filename)
	}

	dirPath := filepath.Dir(filePath)

	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return err
	}

	var data []byte

	if mode == "append" {
		existing, err := os.ReadFile(filePath)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
			existing = append(existing, '\n')
		}
		data = append(existing, content...)
	} else {
		data = content
	}

	if err := validateRecSyntax(data); err != nil {
		return err
	}

	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		return err
	}

	return r.reloadFile(filename, filePath)
}

func (r *Ruleset) reloadFile(filename string, filePath string) error {
	loaders := map[string]func(string) error{
		attributesFile: func(path string) error {
			r.Attributes = make(map[string]*Attribute)
			return loadAttributes(r, path)
		},
		skillsFile: func(path string) error {
			r.Skills = make(map[string]*Skill)
			return loadSkills(r, path)
		},
		speciesFile: func(path string) error {
			r.Species = make(map[string]*Species)
			return loadSpecies(r, path)
		},
		giftsFile: func(path string) error {
			r.Gifts = make(map[string]*Gift)
			return loadGifts(r, path)
		},
		faultsFile: func(path string) error {
			r.Faults = make(map[string]*Fault)
			return loadFaults(r, path)
		},
		powersFile: func(path string) error {
			r.Powers = make(map[string]*Power)
			return loadPowers(r, path)
		},
	}

	if loader, ok := loaders[filename]; ok {
		return loader(filePath)
	}
	return nil
}
