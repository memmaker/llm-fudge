package main

import (
	"encoding/json"
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
	// requestLogDir holds one file per request made to the model, written only
	// under -debug.
	requestLogDir = "log"
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
	// spoken is everything the player has read this turn, in the order they
	// read it. A turn can speak more than once, and the scene is all of it.
	spoken []string
	// requests numbers every call made to the model, for the debug log.
	requests int
	// pendingLogs holds debug transcripts made before the session directory
	// existed — the setup phases and character creation, which are exactly the
	// requests worth reading when the opening of a game goes wrong.
	pendingLogs []pendingLog
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

// pendingLog is one debug transcript waiting for a directory to be written to.
type pendingLog struct {
	name string
	body string
}

// logRequest writes one complete exchange with the model — everything sent,
// everything that came back — to its own file under the session directory. It
// is the whole request, not a summary of it: the debug lines on the terminal
// say what a reply cost and what it called, and every question that outlives
// them is about the text nobody printed.
//
// Debug only. A game not being debugged has no reason to keep a copy of every
// token it has ever sent.
func (s *session) logRequest(messages []llm.Message, tools []llm.Tool, response llm.Response, callErr error) {
	if !debug {
		return
	}
	s.requests++

	var b strings.Builder
	fmt.Fprintf(&b, "# request %04d, turn %d\n\nmodel: %s\nmessages: %d\ntools: %d\n\n",
		s.requests, s.turn, s.model, len(messages), len(tools))

	if len(tools) > 0 {
		b.WriteString("## Tools sent\n\n")
		for _, tool := range tools {
			encoded, err := json.Marshal(tool)
			if err != nil {
				continue
			}
			b.Write(encoded)
			b.WriteString("\n\n")
		}
	}

	b.WriteString("## Messages sent\n\n")
	b.WriteString(transcript(messages))

	b.WriteString("## Response\n\n")
	switch {
	case callErr != nil:
		fmt.Fprintf(&b, "error: %v\n", callErr)
	default:
		b.WriteString(transcript([]llm.Message{response.Message}))
		u := response.Usage
		fmt.Fprintf(&b, "usage: in=%d out=%d total=%d\n", u.InputTokens, u.OutputTokens, u.TotalTokens)
	}

	s.writeLog(fmt.Sprintf("%04d.md", s.requests), b.String())
}

// writeLog queues one transcript and writes everything queued, which is the
// same thing except before the session directory exists.
func (s *session) writeLog(name, body string) {
	s.pendingLogs = append(s.pendingLogs, pendingLog{name: name, body: body})
	s.flushLogs()
}

// flushLogs empties the queue onto disk. It is called again the moment the
// session directory is created, because the requests made before that — the
// setup phases and character creation — are exactly the ones worth reading when
// a game fails to start, and nothing else would ever come back for them.
func (s *session) flushLogs() {
	if s.dir == "" || len(s.pendingLogs) == 0 {
		return
	}
	dir := filepath.Join(s.dir, requestLogDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Println("\nRequest log error:", err)
		return
	}
	for _, held := range s.pendingLogs {
		if err := os.WriteFile(filepath.Join(dir, held.name), []byte(held.body), 0644); err != nil {
			fmt.Println("\nRequest log error:", err)
		}
	}
	s.pendingLogs = nil
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
	s.flushLogs()
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
