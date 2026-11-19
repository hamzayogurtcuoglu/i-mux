package workspace

import "testing"

// Windows' paths ignore case: one folder, typed either way, is one session
// (§7.8).
func TestSessionNameIgnoresCaseOnWindows(t *testing.T) {
	a, b := SessionName(`C:\Users\test\Proj`), SessionName(`c:/users/test/proj`)
	if a != b {
		t.Fatalf("%q != %q", a, b)
	}
	if SessionName(`C:\`) == "" {
		t.Fatal("no name for a drive's root")
	}
}
