//go:build windows

package paths

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRenameWaitsForAFileInUse: Windows does not replace a file another
// handle has open; Rename waits until it is closed.
func TestRenameWaitsForAFileInUse(t *testing.T) {
	dir := t.TempDir()
	dst, src := filepath.Join(dir, "state.json"), filepath.Join(dir, "new.tmp")
	for p, s := range map[string]string{dst: "old", src: "new"} {
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.Open(dst) // without FILE_SHARE_DELETE
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(src, dst); err == nil {
		f.Close()
		t.Skip("Windows replaced a file in use: nothing to wait for")
	}
	time.AfterFunc(200*time.Millisecond, func() { f.Close() })
	if err := Rename(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "new" {
		t.Fatalf("%s holds %q", dst, b)
	}
}
