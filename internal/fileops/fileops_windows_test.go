package fileops

import (
	"errors"
	"testing"
)

// A name may not leave its folder through Windows' separator, nor name a
// drive (§7.8).
func TestCreateRejectsWindowsEscapes(t *testing.T) {
	d := t.TempDir()
	for _, bad := range []string{`..\out`, `a\..\..\out`, `C:out`, `C:\out`} {
		if _, err := Create(d, bad, false); !errors.Is(err, ErrBadName) {
			t.Errorf("Create(%q) = %v, want ErrBadName", bad, err)
		}
	}
}
