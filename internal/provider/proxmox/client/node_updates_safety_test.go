package client

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

func TestNodeUpdatesCacheSideEffectIsNeverRetried(t *testing.T) {
	for _, lostResponse := range []bool{false, true} {
		t.Run(map[bool]string{false: "server error", true: "lost response"}[lostResponse], func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/nodes/fixture/apt/update" {
					t.Errorf("request %s %s", r.Method, r.URL.Path)
				}
				if lostResponse {
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = conn.Close()
					return
				}
				http.Error(w, "fixture failure", http.StatusInternalServerError)
			}))
			defer server.Close()
			c := &Client{baseURL: server.URL, client: httpclient.New()}
			if _, err := c.GetNodeUpdates(t.Context(), "fixture"); err == nil {
				t.Fatal("expected failed observation")
			}
			if calls.Load() != 1 {
				t.Fatalf("side-effecting GET attempted %d times", calls.Load())
			}
		})
	}
}
