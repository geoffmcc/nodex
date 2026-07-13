package output

import (
	"os"
	"strings"
	"unicode/utf8"

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
			// CSI sequences start with [ (0x5B) after ESC.
			// They end with a letter in 0x40-0x7E range.
			if c == 0x5b {
				// CSI introducer found, now look for final byte.
				continue
			}
			if c >= 0x40 && c <= 0x7e {
				inEscape = false
			}
			continue
		}
		result = append(result, c)
	}
	clean := strings.ToValidUTF8(string(result), "�")
	out := make([]rune, 0, len(clean))
	for _, r := range clean {
		if (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			continue
		}
		if r == '\n' || r == '\t' || (r >= 0x20 && r != 0x7f && (r < 0x80 || r >= 0xa0)) {
			out = append(out, r)
		}
	}
	if !utf8.ValidString(string(out)) {
		return strings.ToValidUTF8(string(out), "�")
	}
	return string(out)
}
