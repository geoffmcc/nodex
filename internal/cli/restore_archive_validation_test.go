package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
)

type restoreArchiveProvider struct {
	bareProvider
	items []domain.BackupContentItem
}

func (p *restoreArchiveProvider) BackupContent(context.Context, string, string) ([]domain.BackupContentItem, error) {
	return p.items, nil
}

func TestValidateRestoreArchiveType(t *testing.T) {
	const lxcArchive = "local:backup/vzdump-lxc-9931.tar"
	tests := []struct {
		name     string
		archive  string
		items    []domain.BackupContentItem
		expected string
		wantErr  string
	}{
		{
			name:     "matching subtype",
			archive:  lxcArchive,
			items:    []domain.BackupContentItem{{Volid: lxcArchive, Subtype: "lxc", Format: "tar"}},
			expected: "lxc",
		},
		{
			name:     "reject wrong subtype",
			archive:  lxcArchive,
			items:    []domain.BackupContentItem{{Volid: lxcArchive, Subtype: "lxc", Format: "tar"}},
			expected: "qemu",
			wantErr:  "is a \"lxc\" backup",
		},
		{
			name:     "infer qemu from format",
			archive:  "local:backup/vzdump-qemu-100.vma.zst",
			items:    []domain.BackupContentItem{{Volid: "local:backup/vzdump-qemu-100.vma.zst", Format: "vma"}},
			expected: "qemu",
		},
		{
			name:     "reject archive not in storage inventory",
			archive:  lxcArchive,
			items:    []domain.BackupContentItem{},
			expected: "lxc",
			wantErr:  "was not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &restoreArchiveProvider{items: tt.items}
			err := validateRestoreArchiveType(context.Background(), provider, "pve1", tt.archive, tt.expected)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateRestoreArchiveType() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateRestoreArchiveType() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}
