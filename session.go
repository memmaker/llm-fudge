package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"fudge/prompts"

	"github.com/memmaker/gollum/llm"
)

const (
	// charsheetFile is the whole of the world: the character, the place they
	// stand in, and everything established as true. The DM writes it; nothing
	// parses it. See docs/adr/006.
	charsheetFile = "charsheet.md"
	// charsheetTemplate lives at the repo root rather than in the session
	// directory. It is read once per game to shape the first charsheet, and
	// after that the charsheet is its own template.
	charsheetTemplate = "charsheet_template.md"
	// savesRoot holds one directory per played session.
	savesRoot = "saves"
)

// session is the harness. It owns the session directory, decides what may be
// written into it, and assembles the charsheet block that rides at the tail of
// every request. It holds no world state — that is the charsheet's job.
type session struct {
	dir      string
	writable map[string]bool
	// read tracks which files have been looked at this session. Write refuses to
	// overwrite a file nobody here has seen, which is the only guard against a
	// hallucinated filename destroying the session's only state.
	read map[string]bool

	rng *rand.Rand

	settingName string
	settingDesc string
	adventure   string

	turn          int
	milestones    int
	lastNarration string

	prompts gamePrompts
}

// gamePrompts is the text the turn loop sends that has nowhere to report a
// missing file: the narration demand mid-turn, the header above the sheet on
// every request. They are loaded once at the start of the game phase so an
// absent or malformed prompt file fails there, loudly, rather than turning into
// an empty string three turns in.
type gamePrompts struct {
	narrationDemand string
	emptyReply      string
	charsheetHeader string
	sheetNag        string
	styleMarker     string
}

func loadGamePrompts() (gamePrompts, error) {
	var p gamePrompts
	for _, load := range []struct {
		into *string
		file string
	}{
		{&p.narrationDemand, "turn-narration-demand.md"},
		{&p.emptyReply, "turn-empty-reply.md"},
		{&p.charsheetHeader, "charsheet-header.md"},
		{&p.sheetNag, "sheet-nag.md"},
		{&p.styleMarker, "style-override.md"},
	} {
		text, err := prompts.Fill(load.file, nil)
		if err != nil {
			return gamePrompts{}, err
		}
		*load.into = text
	}
	return p, nil
}

func newSession() *session {
	seed := time.Now().UnixNano()
	return &session{
		writable: map[string]bool{charsheetFile: true},
		read:     map[string]bool{},
		rng:      rand.New(rand.NewSource(seed)),
	}
}

// createDir makes the session directory and is the first tool call of a game.
// It runs after character creation, because the character's name is half the
// directory's name.
func (s *session) createDir(root, character, adventure string) (string, error) {
	if strings.TrimSpace(character) == "" {
		return "", fmt.Errorf("the character has no name")
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%04d-%s-%s", nextSessionNumber(root), slugify(character), slugify(adventure))
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	s.dir = dir
	return dir, nil
}

var sessionNumber = regexp.MustCompile(`^(\d{4})-`)

func nextSessionNumber(root string) int {
	entries, err := os.ReadDir(root)
	if err != nil {
		return 1
	}
	highest := 0
	for _, e := range entries {
		m := sessionNumber.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		if n, _ := strconv.Atoi(m[1]); n > highest {
			highest = n
		}
	}
	return highest + 1
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// allowWrite adds a filename to what the DM may write. The harness calls it for
// the milestone file it has just named; nothing the model says can extend the
// list.
func (s *session) allowWrite(name string) { s.writable[name] = true }

// resolve turns a bare filename into a path inside the session directory, and
// refuses everything else. Paths are not accepted at all — not because "../"
// is the only way out, but because a filename that is only ever a filename has
// no way out to check for.
func (s *session) resolve(name string) (string, error) {
	if name == charsheetTemplate {
		return charsheetTemplate, nil
	}
	if s.dir == "" {
		return "", fmt.Errorf("no session directory yet — call create_session_directory first")
	}
	if name == "" {
		return "", fmt.Errorf("no filename given")
	}
	if name != filepath.Base(name) || strings.ContainsRune(name, filepath.Separator) {
		return "", fmt.Errorf("%q is a path — pass a bare filename, files live in the session directory", name)
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("%q is not a filename", name)
	}
	return filepath.Join(s.dir, name), nil
}

func (s *session) readFile(name string) (string, error) {
	path, err := s.resolve(name)
	if err != nil {
		return "", err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s.read[name] = true
	return string(body), nil
}

func (s *session) writeFile(name, content string) (string, error) {
	path, err := s.checkWritable(name)
	if err != nil {
		return "", err
	}
	// Anything but a plain "it isn't there" is treated as "it is there": the
	// guard exists to stop a hallucinated filename destroying the session's only
	// state, and a Stat that failed for some other reason has not established
	// that there is nothing to destroy.
	if _, err := os.Stat(path); !os.IsNotExist(err) && !s.read[name] {
		return "", fmt.Errorf("%s already exists and has not been read this session — read it first, or edit it instead of overwriting it", name)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	s.read[name] = true
	return fmt.Sprintf("Wrote %s (%d bytes).", name, len(content)), nil
}

func (s *session) editFile(name, oldString, newString string, replaceAll bool) (string, error) {
	path, err := s.checkWritable(name)
	if err != nil {
		return "", err
	}
	if oldString == newString {
		return "", fmt.Errorf("old_string and new_string are identical — nothing to change")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := string(body)

	count := strings.Count(text, oldString)
	switch {
	case count == 0:
		return "", fmt.Errorf("old_string not found in %s — copy the text exactly as it stands, whitespace included", name)
	case count > 1 && !replaceAll:
		return "", fmt.Errorf("old_string matches %d times in %s — add surrounding context to single one out, or pass replace_all", count, name)
	}

	if !replaceAll {
		count = 1
	}
	updated := strings.Replace(text, oldString, newString, count)
	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		return "", err
	}
	s.read[name] = true
	return fmt.Sprintf("Edited %s (%d replacement(s)).", name, count), nil
}

// checkWritable is the whitelist gate both write paths pass through, and it
// returns the resolved path so neither has to resolve it again.
func (s *session) checkWritable(name string) (string, error) {
	if !s.writable[name] {
		return "", fmt.Errorf("%q is not writable — this session may write: %s", name, strings.Join(s.writableNames(), ", "))
	}
	return s.resolve(name)
}

func (s *session) writableNames() []string {
	out := make([]string, 0, len(s.writable))
	for name := range s.writable {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// charsheetPath is where the sheet lives, or "" before the session directory
// exists.
func (s *session) charsheetPath() string {
	if s.dir == "" {
		return ""
	}
	return filepath.Join(s.dir, charsheetFile)
}

// withCharsheet returns the messages a request is actually made of: the history
// as it stands, with the charsheet pinned after it as a single system message.
// The block is assembled for the request and thrown away, so exactly one copy is
// ever present and it can never go stale. Appending it last also leaves the
// growing prefix — ground rules, then chat — byte-identical and cacheable.
func (s *session) withCharsheet(msgs []llm.Message) []llm.Message {
	body, err := os.ReadFile(s.charsheetPath())
	if s.charsheetPath() == "" || err != nil {
		return msgs
	}
	var b strings.Builder
	b.WriteString(s.prompts.charsheetHeader)
	b.WriteString("\n\n")
	b.Write(body)
	out := make([]llm.Message, 0, len(msgs)+1)
	out = append(out, msgs...)
	return append(out, llm.Message{Role: llm.RoleSystem, Content: b.String()})
}
