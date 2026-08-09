package main

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	// Aliased: this package already has a `debug` of its own, the -debug flag.
	buildinfo "runtime/debug"
	"strconv"
	"strings"
	"time"

	rec_files "github.com/memmaker/gollum/rec-files"

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
	// charsheetTemplate is a language-specific prompt asset. It is inlined into
	// the DM prompt to shape the first sheet, and after that the sheet is its own
	// template.
	charsheetTemplate = "charsheet_template.md"
	// requestLogDir holds one pair of files per request made to the model —
	// context-NNN.md to read, context-NNN.json to resume from — written only
	// under -debug. See docs/adr/009.
	requestLogDir = "log"
	// metadataFile is what the session knows about itself before anything is
	// played. It is the only structured file the harness writes for itself, and
	// it is what lets a recording be resumed without being told where it came
	// from.
	metadataFile = "metadata.json"
	// transcriptFile is the game from the player's side: the prose they read and
	// the lines they typed, in order and unwrapped. Written every turn, debug or
	// not, because it is the artefact a test suite reads.
	transcriptFile = "transcript.md"
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
	// serviceTier is OpenAI's processing tier, empty unless llm_model.rec asks
	// for one. Flex is cheaper and slower, which suits a measurement run and
	// not a game being played.
	serviceTier llm.ServiceTier

	settingName   string
	settingDesc   string
	adventure     string
	adventureName string
	// resumedFrom is the context sidecar this session was started from, or "" if
	// it was played from the beginning. It only travels as far as metadata.json.
	resumedFrom string

	turn          int
	milestones    int
	lastNarration string
	// sheetWritten says whether the DM has ever produced a world update block.
	// It is what gates the missing-block note: before the first sheet there is
	// nothing to restate, and asking for one mid-character-creation produces a
	// half-made sheet that then gets copied forward as truth.
	sheetWritten bool
	// pendingSheet is a sheet handed over by `world_update` earlier in the turn,
	// waiting for the scene that turn ends on. It is cleared at the start of
	// every turn, so a sheet never outlives the turn that produced it.
	pendingSheet string
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
	charsheetHeader string
	charsheetNone   string
	charsheetShape  string
	styleMarker     string
	noteMissing     string
	noteSessionEnd  string
	noteForm        string
	sceneDemand     string
}

func loadGamePrompts() (gamePrompts, error) {
	var p gamePrompts
	for _, load := range []struct {
		into *string
		file string
	}{
		{&p.charsheetHeader, "charsheet-header.md"},
		{&p.charsheetNone, "charsheet-none.md"},
		{&p.charsheetShape, charsheetTemplate},
		{&p.styleMarker, "style-override.md"},
		{&p.noteMissing, "note-missing-block.md"},
		{&p.noteSessionEnd, "note-session-end.md"},
		{&p.noteForm, "note-block-form.md"},
		{&p.sceneDemand, "scene-demand.md"},
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
	s.serviceTier = serviceTierFromConfig()
	return nil
}

// serviceTierFromConfig reads the optional `service_tier` field out of the same
// llm_model.rec the client was built from — OpenAI's own parameter name and its
// own values, so a run asks for flex processing by writing `service_tier: flex`
// beside the model.
//
// Read here rather than in InitClientFromHome because that returns a fixed four
// values to every program that uses the library, and one project wanting a
// cheaper tier is not a reason to change what all of them are handed. Providers
// that are not OpenAI drop the field in transport, so passing it always is safe.
//
// A config that is missing, unreadable or silent about the tier means no tier,
// which is exactly today's request.
func serviceTierFromConfig() llm.ServiceTier {
	f, err := os.Open("llm_model.rec")
	if err != nil {
		return ""
	}
	defer f.Close()
	recs := rec_files.Read(f)
	if len(recs) == 0 {
		return ""
	}
	return llm.ServiceTier(recs[0].ToMap()["service_tier"])
}

// sessionMeta is what a session knows about itself before a word is played. It
// is written once, at startup, and read back by -resume: a recording that
// cannot say which setting and adventure it came from cannot be rebuilt against
// today's prompts.
type sessionMeta struct {
	Setting   string `json:"setting"`
	Adventure string `json:"adventure"`
	Language  string `json:"language"`
	Model     string `json:"model"`
	// Commit is the build's revision, `-dirty` when the tree it was built from
	// had uncommitted changes. A measurement is only comparable against another
	// if the prompts were the same, and the prompts are what the commit pins —
	// a run played against a swapped-in prompt file reads as `-dirty` rather
	// than quietly claiming the commit it was branched from.
	Commit string `json:"commit,omitempty"`
	// Branch is where the experiment lives. Two branches can sit on sibling
	// commits and mean entirely different things, so the commit alone does not
	// say what was being tried — the branch's own EXPERIMENT.md does.
	Branch string `json:"branch,omitempty"`
	// Prompts fingerprints the prompt set on disk. It is the identity a
	// measurement is actually comparable under: prompts are read at run time, so
	// two runs can share a commit and not share their prompts.
	Prompts string `json:"prompts,omitempty"`
	Started string `json:"started"`
	// ResumedFrom is the sidecar this session was started from, when it was not
	// started from the beginning. Without it a resumed run reads as an ordinary
	// one that happens to open mid-scene.
	ResumedFrom string `json:"resumed_from,omitempty"`
}

// buildCommit is the revision this binary was built from. The Go toolchain
// stamps it into the build info when building inside a repository, so nothing
// has to shell out to git and a binary carried elsewhere still knows.
func buildCommit() string {
	info, ok := buildinfo.ReadBuildInfo()
	if !ok {
		return ""
	}
	return commitFrom(info.Settings)
}

// commitFrom is the half worth testing: the short revision, marked dirty when
// the tree was modified. Empty when the binary was built outside a repository —
// better to say nothing than to name a commit the run may not have used.
func commitFrom(settings []buildinfo.BuildSetting) string {
	var rev, modified string
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	if rev == "" {
		return ""
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if modified == "true" {
		return rev + "-dirty"
	}
	return rev
}

// currentBranch is the branch being played, or "" when the game is not being
// run out of a working tree. The build info carries the revision but not the
// branch, and the branch is what names the experiment.
func currentBranch() string {
	head, err := os.ReadFile(filepath.Join(".git", "HEAD"))
	if err != nil {
		return ""
	}
	return branchFrom(string(head))
}

// branchFrom reads .git/HEAD. It holds a symbolic ref on a branch and a bare
// revision when detached — and a detached head has no branch to name, so it
// answers with nothing rather than with a commit wearing a branch's slot.
func branchFrom(head string) string {
	head = strings.TrimSpace(head)
	const prefix = "ref: refs/heads/"
	if !strings.HasPrefix(head, prefix) {
		return ""
	}
	return strings.TrimPrefix(head, prefix)
}

// writeMetadata records the session's identity. Called from createDir, because
// the two things happen at the same moment and a directory without it is a
// recording nothing can resume.
func (s *session) writeMetadata(adventure string) error {
	body, err := json.MarshalIndent(sessionMeta{
		Setting:     s.settingName,
		Adventure:   adventure,
		Language:    prompts.Language(),
		Model:       s.model,
		Commit:      buildCommit(),
		Branch:      currentBranch(),
		Prompts:     prompts.Fingerprint(),
		Started:     time.Now().Format(time.RFC3339),
		ResumedFrom: s.resumedFrom,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, metadataFile), append(body, '\n'), 0644)
}

// appendTranscript adds one line to the player's own record of the game. It
// fails quietly: a transcript is for reading afterwards, and interrupting a turn
// over it would cost the player the scene.
func (s *session) appendTranscript(text string) {
	if s.dir == "" || strings.TrimSpace(text) == "" {
		return
	}
	file, err := os.OpenFile(filepath.Join(s.dir, transcriptFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()
	fmt.Fprintf(file, "%s\n\n", strings.TrimSpace(text))
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
	number := s.requests
	s.requests++

	var b strings.Builder
	fmt.Fprintf(&b, "# request %03d, turn %d\n\nmodel: %s\nmessages: %d\ntools: %d\n\n",
		number, s.turn, s.model, len(messages), len(tools))

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

	s.writeLog(fmt.Sprintf("context-%03d.md", number), b.String())

	// The sidecar is the same request as bytes rather than as prose: exactly what
	// went on the wire, so -resume can put a turn back in front of the model
	// without parsing anything a prompt edit could change under it.
	if encoded, err := json.Marshal(messages); err == nil {
		s.writeLog(fmt.Sprintf("context-%03d.json", number), string(encoded))
	}
}

// writeFinalContext closes the recording's last gap. A sidecar holds what was
// sent, so a turn's reply only shows up in the next request's file — and the
// last reply of a session is in no request's file at all.
func (s *session) writeFinalContext(messages []llm.Message) {
	if !debug || s.dir == "" {
		return
	}
	encoded, err := json.Marshal(messages)
	if err != nil {
		return
	}
	s.writeLog("context-final.json", string(encoded))
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
	// Set here rather than at the call sites: this is the sheet's only writer, so
	// the flag cannot drift from the file it stands for.
	s.sheetWritten = true
	log, err := os.OpenFile(filepath.Join(s.dir, charsheetLog), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer log.Close()
	_, err = fmt.Fprintf(log, "--- turn %d ---\n\n%s\n\n", s.turn, sheet)
	return err
}

// createDir makes the session directory. It runs at startup, before a word is
// played: naming it after the character meant waiting for the first world update
// block, and a session that never emits one lost every log it made. The name is
// a counter and the adventure; who was played is in the transcript and the
// charsheet. See docs/adr/009.
func (s *session) createDir(root, adventure string) (string, error) {
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", err
	}
	// An improvised adventure titled in punctuation alone would slug to nothing
	// and leave a directory ending in a bare dash.
	slug := slugify(adventure)
	if slug == "" {
		slug = "session"
	}
	name := fmt.Sprintf("%04d-%s", nextSessionNumber(root), slug)
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	s.dir = dir
	if err := s.writeMetadata(adventure); err != nil {
		return "", err
	}
	s.flushLogs()
	return dir, nil
}

// recording is one moment of a played session, ready to be put back in front of
// the model. Nothing before it is played again: an LLM holds no state, so a turn
// is defined entirely by the context it is handed.
type recording struct {
	meta sessionMeta
	// messages is the conversation alone. The ground rules and the briefing are
	// left out because -resume rebuilds them from today's prompt files — that is
	// the whole point of resuming — and the trailing player line is left out
	// because the script supplies it, which is what makes this turn run again.
	messages []llm.Message
	sheet    string
	turn     int
}

// requestTurn reads the turn number back out of a context log's header.
var requestTurn = regexp.MustCompile(`(?m)^# request \d+, turn (\d+)`)

// loadRecording opens a context sidecar and everything around it: the session it
// belongs to, the turn it was made on, and the charsheet that was current then.
func loadRecording(path string) (recording, error) {
	var rec recording

	body, err := os.ReadFile(path)
	if err != nil {
		return rec, err
	}
	var sent []llm.Message
	if err := json.Unmarshal(body, &sent); err != nil {
		return rec, fmt.Errorf("%s is not a context sidecar: %w", filepath.Base(path), err)
	}
	// Ground rules, briefing, at least one player line: anything shorter is not a
	// turn that can be resumed into.
	if len(sent) < 3 {
		return rec, fmt.Errorf("%s holds %d messages, too few to resume from", filepath.Base(path), len(sent))
	}
	if last := sent[len(sent)-1]; last.Role != llm.RoleUser {
		return rec, fmt.Errorf("%s ends on a %s message, so it is mid-turn — resume from the request that opens a turn", filepath.Base(path), last.Role)
	}
	rec.messages = sent[2 : len(sent)-1]

	dir := filepath.Dir(filepath.Dir(path))
	meta, err := os.ReadFile(filepath.Join(dir, metadataFile))
	if err != nil {
		return rec, fmt.Errorf("no %s beside the recording: %w", metadataFile, err)
	}
	if err := json.Unmarshal(meta, &rec.meta); err != nil {
		return rec, err
	}

	// The readable half of the pair is the only place the turn number is written
	// down. A sidecar without one still resumes: it starts counting from zero and
	// takes the newest sheet there is, which is the best guess available.
	known := false
	if header, err := os.ReadFile(strings.TrimSuffix(path, ".json") + ".md"); err == nil {
		if m := requestTurn.FindStringSubmatch(string(header)); m != nil {
			rec.turn, _ = strconv.Atoi(m[1])
			known = true
		}
	}
	if log, err := os.ReadFile(filepath.Join(dir, charsheetLog)); err == nil {
		// The sheet a turn was played with is the last one written before it.
		at := math.MaxInt
		if known {
			at = rec.turn - 1
		}
		rec.sheet = sheetAtTurn(string(log), at)
	}
	return rec, nil
}

// sheetAtTurn is the charsheet as it stood at the end of a given turn: the last
// block written at or before it, since a turn that emitted none leaves a gap and
// the sheet before the gap is what the next request carried.
func sheetAtTurn(log string, turn int) string {
	// Matched on the headers alone and sliced in between: one expression
	// spanning a whole entry would consume the next header along with it, and
	// every second block would go unseen.
	heads := charsheetTurn.FindAllStringSubmatchIndex(log, -1)
	var sheet string
	for i, head := range heads {
		at, err := strconv.Atoi(log[head[2]:head[3]])
		if err != nil || at > turn {
			continue
		}
		end := len(log)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		sheet = strings.TrimSpace(log[head[1]:end])
	}
	return sheet
}

// charsheetTurn matches one `--- turn N ---` header of the charsheet log.
var charsheetTurn = regexp.MustCompile(`(?m)^--- turn (\d+) ---$`)

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
// as it stands, with the charsheet pinned at the tail. The block is assembled
// for the request and thrown away, so exactly one copy is ever present and it
// can never go stale. Keeping it at the tail also leaves the growing prefix —
// ground rules, then chat — byte-identical and cacheable.
//
// The block is never a second system message: several chat templates
// (llama.cpp's Jinja handling among them) raise "System message must be at the
// beginning" and fail the whole request when one appears. It is folded into the
// last message instead of riding in one of its own wherever that message came
// from the player's side, because the strict-alternation templates — Mistral's
// among them — reject two user messages in a row just as hard. Only when the
// history ends on the model's own words, where the sheet does not belong, does
// it get a message to itself; a user message after an assistant one is legal
// everywhere.
func (s *session) withCharsheet(msgs []llm.Message) []llm.Message {
	// Before the first block there is no sheet, and the tail used to carry
	// nothing at all — which left the one turn that has to produce a sheet as the
	// only turn never asked for one. The empty-handed request sends the template
	// in its place: same position, same demand, an example instead of a state.
	header := s.prompts.charsheetHeader
	body, err := os.ReadFile(s.charsheetPath())
	if s.charsheetPath() == "" || err != nil {
		header, body = s.prompts.charsheetNone, []byte(s.prompts.charsheetShape)
	}
	// Anything after the `---` in the header prompt is sent below the sheet
	// instead of above it: the tail of the request is the last thing the model
	// reads before it writes, and the rules it breaks most need to sit there.
	head, tail, _ := strings.Cut(header, "\n---\n")

	var b strings.Builder
	b.WriteString(head)
	b.WriteString("\n\n")
	b.Write(body)
	if strings.TrimSpace(tail) != "" {
		b.WriteString("\n")
		b.WriteString(tail)
	}

	// The copy is what makes folding safe: llm.Message is a value, so rewriting
	// the last element of out leaves the caller's history untouched.
	out := make([]llm.Message, 0, len(msgs)+1)
	out = append(out, msgs...)
	if last := len(out) - 1; last >= 0 && (out[last].Role == llm.RoleUser || out[last].Role == llm.RoleTool) {
		out[last].Content = strings.TrimRight(out[last].Content, "\n") + "\n\n" + b.String()
		return out
	}
	return append(out, llm.Message{Role: llm.RoleUser, Content: b.String()})
}
