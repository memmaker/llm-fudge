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

func LoadRuleset(path string) (*Ruleset, error) {
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
	records, err := readRecFile(path)
	if err != nil {
		return err
	}
	for _, record := range records {
		data := record.ToMap("|")
		slug := strings.ToLower(data["slug"])
		if slug == "" {
			return fmt.Errorf("attribute missing slug in %s", path)
		}
		if !slugPattern.MatchString(slug) {
			return fmt.Errorf("invalid attribute slug: %s", data["slug"])
		}
		r.Attributes[slug] = &Attribute{
			Slug:        slug,
			DisplayName: data["display_name"],
			Description: data["description"],
		}
	}
	return nil
}

func loadSkills(r *Ruleset, path string) error {
	records, err := readRecFile(path)
	if err != nil {
		return err
	}
	for _, record := range records {
		data := record.ToMap("|")
		slug := strings.ToLower(data["slug"])
		if slug == "" {
			return fmt.Errorf("skill missing slug in %s", path)
		}
		if !slugPattern.MatchString(slug) {
			return fmt.Errorf("invalid skill slug: %s", data["slug"])
		}
		r.Skills[slug] = &Skill{
			Slug:        slug,
			DisplayName: data["display_name"],
			Description: data["description"],
		}
	}
	return nil
}

func loadSpecies(r *Ruleset, path string) error {
	records, err := readRecFile(path)
	if err != nil {
		return err
	}
	for _, record := range records {
		data := record.ToMap("|")
		slug := strings.ToLower(data["slug"])
		if slug == "" {
			return fmt.Errorf("species missing slug in %s", path)
		}
		if !slugPattern.MatchString(slug) {
			return fmt.Errorf("invalid species slug: %s", data["slug"])
		}
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
	}
	return nil
}

func loadGifts(r *Ruleset, path string) error {
	records, err := readRecFile(path)
	if err != nil {
		return err
	}
	for _, record := range records {
		data := record.ToMap("|")
		slug := strings.ToLower(data["slug"])
		if slug == "" {
			return fmt.Errorf("gift missing slug in %s", path)
		}
		if !slugPattern.MatchString(slug) {
			return fmt.Errorf("invalid gift slug: %s", data["slug"])
		}
		r.Gifts[slug] = &Gift{
			Slug:        slug,
			DisplayName: data["display_name"],
			Description: data["description"],
		}
	}
	return nil
}

func loadFaults(r *Ruleset, path string) error {
	records, err := readRecFile(path)
	if err != nil {
		return err
	}
	for _, record := range records {
		data := record.ToMap("|")
		slug := strings.ToLower(data["slug"])
		if slug == "" {
			return fmt.Errorf("fault missing slug in %s", path)
		}
		if !slugPattern.MatchString(slug) {
			return fmt.Errorf("invalid fault slug: %s", data["slug"])
		}
		r.Faults[slug] = &Fault{
			Slug:        slug,
			DisplayName: data["display_name"],
			Description: data["description"],
		}
	}
	return nil
}

func loadPowers(r *Ruleset, path string) error {
	records, err := readRecFile(path)
	if err != nil {
		return err
	}
	for _, record := range records {
		data := record.ToMap("|")
		slug := strings.ToLower(data["slug"])
		if slug == "" {
			return fmt.Errorf("power missing slug in %s", path)
		}
		if !slugPattern.MatchString(slug) {
			return fmt.Errorf("invalid power slug: %s", data["slug"])
		}
		r.Powers[slug] = &Power{
			Slug:        slug,
			DisplayName: data["display_name"],
			Description: data["description"],
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
		return fmt.Errorf("ruleset is frozen")
	}

	if !validFiles[filename] {
		return fmt.Errorf("invalid ruleset file: %s", filename)
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

	// Reload the affected file
	switch filename {
	case attributesFile:
		r.Attributes = make(map[string]*Attribute)
		if err := loadAttributes(r, filePath); err != nil {
			return err
		}
	case skillsFile:
		r.Skills = make(map[string]*Skill)
		if err := loadSkills(r, filePath); err != nil {
			return err
		}
	case speciesFile:
		r.Species = make(map[string]*Species)
		if err := loadSpecies(r, filePath); err != nil {
			return err
		}
	case giftsFile:
		r.Gifts = make(map[string]*Gift)
		if err := loadGifts(r, filePath); err != nil {
			return err
		}
	case faultsFile:
		r.Faults = make(map[string]*Fault)
		if err := loadFaults(r, filePath); err != nil {
			return err
		}
	case powersFile:
		r.Powers = make(map[string]*Power)
		if err := loadPowers(r, filePath); err != nil {
			return err
		}
	}

	return nil
}
