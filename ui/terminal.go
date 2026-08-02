package ui

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// The cursor and screen controls are for a live terminal. A scripted run's
// output is a transcript to read or diff, so they write nothing there.

func ClearScreen() {
	if !scripted() {
		fmt.Print("\033[H\033[2J")
	}
}

func HideCursor() {
	if !scripted() {
		fmt.Print("\033[?25l")
	}
}

func ShowCursor() {
	if !scripted() {
		fmt.Print("\033[?25h")
	}
}

// ScrollLastLineToTop scrolls the buffer until the line directly above the
// cursor — the line the player just entered — sits at the top of the viewport,
// so the reply that follows is read from the first row down. The newlines do the
// scrolling, which keeps everything above in scrollback; the cursor-up puts the
// cursor back where it was, and whatever prints next overwrites the blanks.
func ScrollLastLineToTop() {
	if scripted() {
		return
	}
	_, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || height < 3 {
		return
	}
	n := height - 2
	fmt.Print(strings.Repeat("\n", n))
	fmt.Printf("\033[%dA", n)
}

func TerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return 80
	}
	return width
}

func WrapText(text string, width int) string {
	var wrapped []string
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			wrapped = append(wrapped, "")
			continue
		}
		wrapped = append(wrapped, wrapLine(line, width)...)
	}
	return strings.Join(wrapped, "\n")
}

func wrapLine(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}

	var wrapped []string
	var current string
	for _, word := range strings.Fields(line) {
		for len([]rune(word)) > width {
			if current != "" {
				wrapped = append(wrapped, current)
				current = ""
			}
			runes := []rune(word)
			wrapped = append(wrapped, string(runes[:width]))
			word = string(runes[width:])
		}
		if current == "" {
			current = word
			continue
		}
		if len([]rune(current))+1+len([]rune(word)) <= width {
			current += " " + word
			continue
		}
		wrapped = append(wrapped, current)
		current = word
	}
	if current != "" {
		wrapped = append(wrapped, current)
	}
	return wrapped
}

func DisplayContextUsage(tokens int, debug bool) {
	if !debug || tokens <= 0 {
		return
	}
	fmt.Printf("\033[90m[context: %d tokens]\033[0m\n", tokens)
}
