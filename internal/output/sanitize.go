package output

import (
	"os"

	"golang.org/x/term"
)

// isTerminal checks if w is connected to a terminal.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// SanitizeTerminal strips ANSI escape sequences from input.
// This prevents terminal injection attacks from malicious data.
func SanitizeTerminal(s string) string {
	// Remove CSI sequences: ESC [ ... final_byte
	result := make([]byte, 0, len(s))
	inEscape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 0x1b { // ESC
			inEscape = true
			continue
		}
		if inEscape {
			// CSI sequences end with a letter in 0x40-0x7E range.
			if c >= 0x40 && c <= 0x7e {
				inEscape = false
			}
			continue
		}
		result = append(result, c)
	}
	return string(result)
}
