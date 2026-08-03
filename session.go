package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"fudge/prompts"

	"github.com/memmaker/gollum/llm"
)

const (
	// charsheetFile is the whole of the world: the character, the place they
	// stand in, and everything established as true. The DM emits it inside the
	// world update block and the harness writes it; nothing parses it beyond the
	// character's name. See docs/adr/006 and docs/adr/007.
	charsheetFile = "charsheet.md"
	// charsheetLog is every accepted block, appended in order. The sheet itself
	// only ever shows the latest state, so a fact that quietly stopped being
	// restated would leave no trace anywhere else.
	charsheetLog = "charsheet-log.md"
	// charsheetTemplate lives at the repo root rather than in the session
	// directory. It is inlined into the DM prompt to shape the first sheet, and
	// after that the sheet is its own template.
	charsheetTemplate = "charsheet_template.md"
	// savesRoot holds one directory per played session.
	savesRoot = "saves"
	// narrationFile and imagePromptFile are written in the working directory for
	// whatever wants the current scene without parsing the terminal. Each turn
	// replaces them; the run's history is the session directory's job.
	narrationFile   = "narration.txt"
	imagePromptFile = "img-gen.md"
)

// session is the harness. It owns the session directory, writes the charsheet
// the DM hands it, and assembles the block that rides at the tail of every
// request. It holds no world state — that is the charsheet's job.
type session struct {
	dir string

	rng *rand.Rand

	client *llm.Client
	model  string

	settingName   string
	settingDesc   string
	adventure     string
	adventureName string

	turn          int
	milestones    int
	lastNarration string
	// carried is the harness's only way of correcting the DM, and it never
	// spends a request to do it: whatever it has to say rides in on the next
	// player line. The player never sees it.
	carried string

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
	styleMarker     string
	noteMissing     string
	noteForm        string
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
		{&p.styleMarker, "style-override.md"},
		{&p.noteMissing, "note-missing-block.md"},
		{&p.noteForm, "note-block-form.md"},
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
	return &session{rng: rand.New(rand.NewSource(seed))}
}

// connect opens the one LLM client the whole run uses. It was opened three
// separate times before, once per setup phase and once for play. Kept apart
// from newSession so a test can build a session without reaching for a
// provider config that is not there.
func (s *session) connect() error {
	client, model, _, err := llm.InitClientFromHome()
	if err != nil {
		return err
	}
	s.client, s.model = client, model
	return nil
}

// characterName is the one thing the harness reads out of the sheet, and only
// on the first one: the session directory is named after the character, so
// `saves/` is browsable. Everything else in the file is prose for the DM.
var characterName = regexp.MustCompile(`(?im)^\s*[-*]?\s*(?:\*\*)?Name(?:\*\*)?\s*:\s*(.+?)\s*$`)

// deriveDir opens the session directory from the first accepted block. Doing it
// here rather than in a tool call is what lets the opening turn look like every
// other turn: prose plus block, in one response, with no order to get wrong.
func (s *session) deriveDir(sheet string) error {
	// A sheet with no name line still gets a directory. The block is the only
	// copy of the world at this point, and losing it over a missing heading
	// would be the expensive half of a cheap mistake.
	character := "unnamed"
	if m := characterName.FindStringSubmatch(sheet); m != nil {
		character = m[1]
	}
	_, err := s.createDir(savesRoot, character, s.adventureName)
	return err
}

// writeCharsheet is the sheet's only writer. Every accepted block is also
// appended to a log beside it: the sheet shows the latest state and nothing
// else, so a line that quietly stopped being restated would otherwise vanish
// without trace.
func (s *session) writeCharsheet(sheet string) error {
	if err := os.WriteFile(s.charsheetPath(), []byte(sheet+"\n"), 0644); err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(s.dir, charsheetLog), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer log.Close()
	_, err = fmt.Fprintf(log, "--- turn %d ---\n\n%s\n\n", s.turn, sheet)
	return err
}

// createDir makes the session directory. It runs on the first block, because
// the character's name is half the directory's name and the block is where the
// character first exists.
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
