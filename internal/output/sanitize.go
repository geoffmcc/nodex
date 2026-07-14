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
// Handles CSI, OSC, DCS, APC, PM, and other escape sequence families.
func SanitizeTerminal(s string) string {
	// Remove all escape sequences: ESC-initiated byte sequences.
	result := make([]byte, 0, len(s))
	i := 0
	for i < len(s) {
		c := s[i]
		if c == 0x1b { // ESC
			i++
			if i >= len(s) {
				// Lone ESC at end of string — drop it.
				break
			}
			next := s[i]
			switch {
			case next == '[': // CSI: ESC [ <params> <intermediate> <final>
				i++
				// Skip parameter bytes (0x30-0x3F), intermediate bytes (0x20-0x2F),
				// and stop at the final byte (0x40-0x7E).
				for i < len(s) {
					b := s[i]
					if b >= 0x40 && b <= 0x7e {
						i++ // consume final byte
						break
					}
					i++
				}
			case next == ']' || next == 'P' || next == '_' || next == '^':
				// OSC (ESC ]), DCS (ESC P), APC (ESC _), PM (ESC ^).
				// Payload continues until ST (ESC \) or BEL (0x07).
				i++
				for i < len(s) {
					if s[i] == 0x07 { // BEL — OSC terminator
						i++
						break
					}
					if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' { // ESC \ — ST
						i += 2
						break
					}
					i++
				}
			default:
				// Other single-character escape sequences (e.g., ESC A, ESC M).
				// Just skip the ESC + the one character.
				i++
			}
			continue
		}
		result = append(result, c)
		i++
	}
	clean := strings.ToValidUTF8(string(result), "\uFFFD")
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
		return strings.ToValidUTF8(string(out), "\uFFFD")
	}
	return string(out)
}
