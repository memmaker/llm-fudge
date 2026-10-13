package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/chzyer/readline"
	markterm "github.com/memmaker/gollum/mark-term"
	"golang.org/x/term"
)

// TypeYourAnswer is the menu's last entry: an opt-out, not an option. Picking
// it means the player wants to say something the options do not cover.
const TypeYourAnswer = "[Type your answer]"

// script is the queue a -script run answers every prompt from: one line per
// answer, in the order the game asks for them. It is nil in an interactive
// session, and scripted() is what the terminal control sequences check — a
// transcript meant to be read or diffed should not be threaded with cursor
// moves and screen clears.
var script []string

func scripted() bool { return script != nil }

// LoadScript reads the answers a run will play. Blank lines and # comments are
// dropped so a script can be annotated; everything else is an answer verbatim,
// leading and trailing space included, because a scenario description is one.
func LoadScript(r io.Reader) error {
	script = []string{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		script = append(script, line)
	}
	return sc.Err()
}

// nextScripted pops the next scripted answer. Running out is the end of the
// run, not an error: a script says everything the player had to say.
func nextScripted() (string, bool) {
	if len(script) == 0 {
		return "", false
	}
	line := script[0]
	script = script[1:]
	return line, true
}

// inputLog records what the player answered, one line per answer, in exactly
// the format LoadScript reads back — a debug run's log is a script that replays
// the session. It is written unbuffered so a crash still leaves the input that
// caused it on disk, which is the whole point.
var inputLog io.Writer

func LogInputs(w io.Writer) { inputLog = w }

func logInput(line string) {
	if inputLog != nil {
		fmt.Fprintln(inputLog, line)
	}
}

// chose logs a menu pick the way a script spells it: naming the option. Picking
// "type your answer" is not written, because a script does not spell it either —
// the free text that follows is what selects it, and the prompt asking for that
// text logs the line itself.
func chose(opt string) string {
	if opt != TypeYourAnswer {
		logInput(opt)
	}
	return opt
}

// GetInput never returns a blank line: an empty answer is not an answer, and it
// goes straight back to the prompt without costing a request. The guard lives
// here rather than at the call sites so it covers the setup questions too,
// where a bare Enter used to be sent to the model as if it were a reply.
func GetInput(rl *readline.Instance, prompt string) (string, error) {
	for {
		line, err := readOnce(rl, prompt)
		if err != nil {
			return line, err
		}
		if strings.TrimSpace(line) != "" {
			// Logged here and not in readOnce so the blank answers the loop
			// swallows stay out: LoadScript drops blank lines anyway, and a log
			// that replays has to hold only what was actually answered.
			logInput(line)
			return line, nil
		}
	}
}

func readOnce(rl *readline.Instance, prompt string) (string, error) {
	if scripted() {
		line, ok := nextScripted()
		if !ok {
			return "", io.EOF
		}
		fmt.Printf("%s%s\n", prompt, line)
		return line, nil
	}
	if rl == nil {
		// A caller with no instance of its own — the user_choice tool, which is
		// reached from the tool layer — gets a throwaway one. Cheaper than
		// threading the game's readline through every tool handler.
		var err error
		if rl, err = readline.New(""); err != nil {
			return "", err
		}
		defer rl.Close()
	}
	ShowCursor()
	rl.SetPrompt(prompt)
	line, err := rl.Readline()
	HideCursor()
	return line, err
}

func readKey() (string, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", fmt.Errorf("failed to set raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	buf := make([]byte, 3)
	n, err := os.Stdin.Read(buf)
	if err != nil {
		return "", err
	}

	if n >= 1 {
		switch buf[0] {
		case 10, 13:
			return "enter", nil
		case 27:
			if n >= 3 && buf[1] == '[' {
				switch buf[2] {
				case 'A':
					return "up", nil
				case 'B':
					return "down", nil
				case 'C':
					return "right", nil
				case 'D':
					return "left", nil
				}
			}
			return "escape", nil
		}
	}
	return "", nil
}

func ShowChoiceMenu(options []string) (string, error) {
	allOptions := append(options, TypeYourAnswer)
	selected := 0

	// A scripted run picks by naming the option. A line that names none of them
	// is the answer the menu's own "type your answer" branch is about to ask
	// for, so it goes back on the queue rather than being spent here.
	if scripted() {
		line, ok := nextScripted()
		if !ok {
			return "", io.EOF
		}
		for _, opt := range allOptions {
			if line == opt {
				fmt.Printf("[menu] %s\n", opt)
				return chose(opt), nil
			}
		}
		script = append([]string{line}, script...)
		fmt.Printf("[menu] %s\n", TypeYourAnswer)
		return TypeYourAnswer, nil
	}

	rl, err := readline.New("")
	if err != nil {
		return "", err
	}
	defer rl.Close()

	fmt.Println()
	firstDraw := true

	for {
		if !firstDraw {
			// Calculate total lines to clear (each option may wrap)
			totalLines := 2 // 1 for blank line before, 1 for help text at end
			for _, opt := range allOptions {
				wrapped := markterm.Wrap(opt, TerminalWidth()-4)
				lineCount := strings.Count(wrapped, "\n") + 1
				totalLines += lineCount
			}
			fmt.Printf("\033[%dA\033[J", totalLines)
		}

		for i, opt := range allOptions {
			wrapped := markterm.Wrap(opt, TerminalWidth()-4)
			lines := strings.Split(wrapped, "\n")

			for j, line := range lines {
				if i == selected {
					if j == 0 {
						fmt.Printf("  \033[94m> %s\033[0m\n", line)
					} else {
						fmt.Printf("    %s\n", line)
					}
				} else {
					fmt.Printf("    %s\n", line)
				}
			}
		}
		fmt.Printf("\n  \033[90m↑↓ to navigate, Enter to select\033[0m\n")

		firstDraw = false

		key, err := readKey()
		if err != nil {
			return "", err
		}

		switch key {
		case "up":
			if selected > 0 {
				selected--
			} else {
				selected = len(allOptions) - 1
			}
		case "down":
			if selected < len(allOptions)-1 {
				selected++
			} else {
				selected = 0
			}
		case "enter":
			fmt.Println()
			return chose(allOptions[selected]), nil
		case "escape":
			fmt.Println()
			return chose(allOptions[len(allOptions)-1]), nil
		}
	}
}
