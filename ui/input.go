package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/chzyer/readline"
	"golang.org/x/term"
)

const typeYourAnswer = "[Type your answer]"

func TypeYourAnswer() string {
	return typeYourAnswer
}

func GetInput(rl *readline.Instance, prompt string) (string, error) {
	ShowCursor()
	rl.SetPrompt(prompt)
	line, err := rl.Readline()
	HideCursor()
	return line, err
}

func ReadKey() (string, error) {
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
	allOptions := append(options, typeYourAnswer)
	selected := 0

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
				wrapped := WrapText(opt, TerminalWidth()-4)
				lineCount := strings.Count(wrapped, "\n") + 1
				totalLines += lineCount
			}
			fmt.Printf("\033[%dA\033[J", totalLines)
		}

		for i, opt := range allOptions {
			wrapped := WrapText(opt, TerminalWidth()-4)
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

		key, err := ReadKey()
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
			return allOptions[selected], nil
		case "escape":
			fmt.Println()
			return allOptions[len(allOptions)-1], nil
		}
	}
}
