package cli

import "testing"

func TestParseReplicationJobID(t *testing.T) {
	tests := []struct {
		id        string
		wantGuest int
		wantErr   bool
	}{
		{id: "9924-0", wantGuest: 9924},
		{id: "100-999999999", wantGuest: 100},
		{id: "99-0", wantErr: true},
		{id: "9924", wantErr: true},
		{id: "9924-vm", wantErr: true},
		{id: "9924-1000000000", wantErr: true},
		{id: "9924-0-extra", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got, err := parseReplicationJobID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseReplicationJobID(%q) error = %v, wantErr %t", tt.id, err, tt.wantErr)
			}
			if err == nil && got != tt.wantGuest {
				t.Fatalf("parseReplicationJobID(%q) = %d, want %d", tt.id, got, tt.wantGuest)
			}
		})
	}
}
