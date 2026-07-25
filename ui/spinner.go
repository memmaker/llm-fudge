package ui

import (
	"fmt"
	"time"
)

func ShowLoadingSpinner(duration time.Duration, done <-chan struct{}) {
	brailleChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	frame := 0
	for {
		select {
		case <-done:
			fmt.Print("\r\033[0K")
			return
		case <-ticker.C:
			fmt.Printf("\r%s  ", brailleChars[frame%len(brailleChars)])
			frame++
		}
	}
}
