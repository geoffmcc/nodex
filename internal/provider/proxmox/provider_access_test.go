package proxmox

import "testing"

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"admins", []string{"admins"}},
		{"admins,ops", []string{"admins", "ops"}},
		{" admins , ops ", []string{"admins", "ops"}},
		{"admins,,ops", []string{"admins", "ops"}},
		{",", nil},
	}
	for _, tt := range tests {
		got := splitCSV(tt.in)
		if len(got) != len(tt.want) {
			t.Errorf("splitCSV(%q) = %#v, want %#v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitCSV(%q) = %#v, want %#v", tt.in, got, tt.want)
				break
			}
		}
	}
}
