package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/geoffmcc/nodex/internal/output"
)

func writeStatusGuestInventory(w io.Writer, indent, label, sectionState string, total int, guests []statusGuest, showNode bool) error {
	if sectionState == "" {
		sectionState = "unsupported"
	}
	if sectionState != "available" {
		_, err := fmt.Fprintf(w, "%s%s: %s\n", indent, label, statusFieldText(sectionState))
		return err
	}
	if total == 0 {
		_, err := fmt.Fprintf(w, "%s%s: none\n", indent, label)
		return err
	}
	const maxRows = 10
	shown := min(len(guests), maxRows)
	if _, err := fmt.Fprintf(w, "%s%s (showing %d of %d):\n", indent, label, shown, total); err != nil {
		return err
	}
	for _, guest := range guests[:shown] {
		id := guest.ID
		if slash := strings.LastIndexByte(id, '/'); slash >= 0 {
			id = id[slash+1:]
		}
		name := guest.Name
		if name == "" {
			name = "(unnamed)"
		}
		state := guest.Status
		if state == "" {
			state = "unknown"
		}
		if showNode {
			if _, err := fmt.Fprintf(w, "%s  %s  %s  %s  [%s]\n", indent, statusFieldText(id), statusFieldText(name), statusFieldText(state), statusFieldText(guest.Node)); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "%s  %s  %s  %s\n", indent, statusFieldText(id), statusFieldText(name), statusFieldText(state)); err != nil {
				return err
			}
		}
	}
	if total > shown {
		if _, err := fmt.Fprintf(w, "%s  ... %d more; use the list command for all guests\n", indent, total-shown); err != nil {
			return err
		}
	}
	return nil
}

func statusFieldText(value string) string {
	value = output.SanitizeTerminal(value)
	return strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(value)
}
