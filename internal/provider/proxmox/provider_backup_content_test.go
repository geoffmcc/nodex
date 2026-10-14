package proxmox

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

// Mixed storage content must not leak ISOs or container templates into a
// backup listing, and the useful backup metadata (VMID, creation time, and
// subtype) must survive the mapping.
func TestBackupContentFiltersNonBackupObjects(t *testing.T) {
	var gotQuery string
	p := connectForStorageTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/proxmox/storage/local/content":
			gotQuery = r.URL.RawQuery
			_, _ = fmt.Fprint(w, `{"data":[
				{"content":"backup","volid":"local:backup/vzdump-lxc-9100-2026_10_14-10_27_29.tar","size":582891520,"format":"tar","vmid":9100,"ctime":1760456849,"subtype":"lxc"},
				{"content":"iso","volid":"local:iso/debian-13.iso","size":700000000,"format":"iso"},
				{"content":"vztmpl","volid":"local:vztmpl/debian-13-standard_13.1-2_amd64.tar.zst","size":120000000,"format":"tzst"}
			]}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	})

	items, err := p.BackupContent(context.Background(), "proxmox", "local")
	if err != nil {
		t.Fatalf("BackupContent: %v", err)
	}
	if gotQuery != "content=backup" {
		t.Errorf("query = %q, want content=backup", gotQuery)
	}
	if len(items) != 1 {
		t.Fatalf("len = %d, want 1 backup item: %+v", len(items), items)
	}
	got := items[0]
	if got.Volid != "local:backup/vzdump-lxc-9100-2026_10_14-10_27_29.tar" {
		t.Errorf("volid = %q", got.Volid)
	}
	if got.VMID != 9100 {
		t.Errorf("vmid = %d, want 9100", got.VMID)
	}
	if got.Subtype != "lxc" {
		t.Errorf("subtype = %q, want lxc", got.Subtype)
	}
	if got.Ctime != 1760456849 {
		t.Errorf("ctime = %d, want 1760456849", got.Ctime)
	}
	if got.Size != 582891520 || got.Format != "tar" {
		t.Errorf("size/format = %d/%q", got.Size, got.Format)
	}
}

// A backend that ignores the content filter still must not leak non-backup
// objects, because the client-side filter is the guarantee.
func TestBackupContentDropsNonBackupWhenFilterIgnored(t *testing.T) {
	p := connectForStorageTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/proxmox/storage/local/content" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"data":[
			{"content":"iso","volid":"local:iso/debian-13.iso","size":700000000},
			{"content":"backup","volid":"local:backup/vzdump-qemu-100-2026_10_14-10_27_29.vma.zst","size":123,"format":"vma.zst","vmid":100,"subtype":"qemu"},
			{"content":"vztmpl","volid":"local:vztmpl/debian-13.tar.zst","size":120000000}
		]}`)
	})

	items, err := p.BackupContent(context.Background(), "proxmox", "local")
	if err != nil {
		t.Fatalf("BackupContent: %v", err)
	}
	if len(items) != 1 || items[0].VMID != 100 {
		t.Fatalf("items = %+v, want only the VM 100 backup", items)
	}
}
