package proxmox

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"
)

// A task log request must always be bounded: the provider requests a default
// limit when the caller gives none, and it caps an over-large explicit limit so
// a verbose task cannot make the client download an unbounded log.
func TestTaskLogBoundsRequestedLimit(t *testing.T) {
	var gotLimit string
	handler := func(w http.ResponseWriter, r *http.Request) {
		gotLimit = r.URL.Query().Get("limit")
		_, _ = fmt.Fprint(w, `{"data":[{"n":1,"t":"INFO: start"}]}`)
	}

	cases := []struct {
		name      string
		requested int
		want      int
	}{
		{"default when zero", 0, defaultTaskLogLimit},
		{"explicit passes through", 10, 10},
		{"over-large is capped", maxTaskLogLimit + 5000, maxTaskLogLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := connectForStorageTest(t, handler)
			lines, err := p.TaskLog(context.Background(), "proxmox", "UPID:proxmox/00012345/0", tc.requested)
			if err != nil {
				t.Fatalf("TaskLog: %v", err)
			}
			if gotLimit != strconv.Itoa(tc.want) {
				t.Errorf("requested limit = %q, want %d", gotLimit, tc.want)
			}
			if len(lines) != 1 || lines[0].Text != "INFO: start" {
				t.Errorf("lines = %+v", lines)
			}
		})
	}
}
