package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"fudge/recfile"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// idPattern is what an id the DM minted has to look like. It is stricter than
// slugPattern, which only ever had to accept what slug() produced: an id the
// engine derives cannot start or end with a hyphen or double one up, and an id
// the DM writes has to be held to the shape it is reading everywhere else.
var idPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidID reports whether an id the DM proposed is well formed. The engine no
// longer mints ids for entities the DM introduces — it validates them, and this
// is the syntactic half of that.
func ValidID(id string) bool { return idPattern.MatchString(id) }

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

type SettingInfos struct {
	Slug        string
	Path        string
	Description string
	Attributes  map[string]*Attribute
	Skills      map[string]*Skill
	Species     map[string]*Species
	Gifts       map[string]*Gift
	Faults      map[string]*Fault
	Powers      map[string]*Power
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
	attributesFile  = "attributes.rec"
	skillsFile      = "skills.rec"
	speciesFile     = "species.rec"
	giftsFile       = "gifts.rec"
	faultsFile      = "faults.rec"
	powersFile      = "powers.rec"
	descriptionFile = "description.md"
	adventuresDir   = "adventures"
)

var validFiles = map[string]bool{
	attributesFile:  true,
	skillsFile:      true,
	speciesFile:     true,
	giftsFile:       true,
	faultsFile:      true,
	powersFile:      true,
	descriptionFile: true,
}

func LoadSetting(path string) (*SettingInfos, error) {
	r := &SettingInfos{
		Slug:       filepath.Base(path),
		Path:       path,
		Attributes: make(map[string]*Attribute),
		Skills:     make(map[string]*Skill),
		Species:    make(map[string]*Species),
		Gifts:      make(map[string]*Gift),
		Faults:     make(map[string]*Fault),
		Powers:     make(map[string]*Power),
	}

	if desc, err := os.ReadFile(filepath.Join(path, "description.md")); err == nil {
		r.Description = string(desc)
	} else if !os.IsNotExist(err) {
		return nil, err
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

func loadAttributes(r *SettingInfos, path string) error {
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

func loadSkills(r *SettingInfos, path string) error {
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

func loadSpecies(r *SettingInfos, path string) error {
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

func loadGifts(r *SettingInfos, path string) error {
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

func loadFaults(r *SettingInfos, path string) error {
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

func loadPowers(r *SettingInfos, path string) error {
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

func (r *SettingInfos) WriteFile(filename string, mode string, content []byte) error {
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

	// description.md is prose, not a rec file
	if filename != descriptionFile {
		if err := validateRecSyntax(data); err != nil {
			return err
		}
	}

	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		return err
	}

	return r.reloadFile(filename, filePath)
}

// WriteAdventure persists an improvised adventure in the setting's adventures
// directory so it can be replayed later. The title is slugified into the
// filename; an existing adventure of the same name is never overwritten, a
// numbered variant is written instead. Returns the path written to.
func (r *SettingInfos) WriteAdventure(title string, content []byte) (string, error) {
	name := slug(title)
	if !slugPattern.MatchString(name) {
		return "", fmt.Errorf("adventure title must contain letters or digits: %q", title)
	}
	if len(content) == 0 {
		return "", fmt.Errorf("adventure content is empty")
	}

	dirPath := filepath.Join(r.Path, adventuresDir)
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return "", err
	}

	filePath := filepath.Join(dirPath, name+".md")
	for variant := 2; ; variant++ {
		_, err := os.Stat(filePath)
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			return "", err
		}
		filePath = filepath.Join(dirPath, fmt.Sprintf("%s-%d.md", name, variant))
	}

	if !strings.HasSuffix(string(content), "\n") {
		content = append(content, '\n')
	}
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		return "", err
	}
	return filePath, nil
}

func (r *SettingInfos) reloadFile(filename string, filePath string) error {
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
		descriptionFile: func(path string) error {
			desc, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			r.Description = string(desc)
			return nil
		},
	}

	if loader, ok := loaders[filename]; ok {
		return loader(filePath)
	}
	return nil
}

func (r *SettingInfos) ToString() string {
	var sb strings.Builder

	if r.Description != "" {
		sb.WriteString(r.Description)
		sb.WriteString("\n\n")
	}

	sb.WriteString("SPECIES (closed list — use ONLY these, never a genre default like elf, dwarf or orc unless it is named below):\n")
	keys := make([]string, 0, len(r.Species))
	for k := range r.Species {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, species := range keys {
		sb.WriteString(fmt.Sprintf("  - %s\n", species))
	}

	sb.WriteString("\nATTRIBUTES:\n")
	keys = keys[:0]
	for k := range r.Attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, attr := range keys {
		sb.WriteString(fmt.Sprintf("  - %s\n", attr))
	}

	sb.WriteString("\nSKILLS:\n")
	keys = keys[:0]
	for k := range r.Skills {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, skill := range keys {
		sb.WriteString(fmt.Sprintf("  - %s\n", skill))
	}

	sb.WriteString("\nGIFTS:\n")
	keys = keys[:0]
	for k := range r.Gifts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, gift := range keys {
		sb.WriteString(fmt.Sprintf("  - %s\n", gift))
	}

	sb.WriteString("\nFAULTS:\n")
	keys = keys[:0]
	for k := range r.Faults {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, fault := range keys {
		sb.WriteString(fmt.Sprintf("  - %s\n", fault))
	}

	return sb.String()
}
