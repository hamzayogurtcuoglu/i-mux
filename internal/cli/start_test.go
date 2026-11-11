package cli

import "testing"

// TestStartPick (D49): imux started in an open project attaches; elsewhere
// it asks when projects are open or the last session can be restored, and
// in the home folder; "this folder" is offered except for the home folder.
func TestStartPick(t *testing.T) {
	const home = "/home/u"
	open := map[string]bool{"/home/u/code/api": true}
	for _, c := range []struct {
		cwd        string
		open       map[string]bool
		restorable int
		here       string
		ask        bool
	}{
		{"/home/u/code/api", open, 3, "", false},                // an open project: attach
		{"/home/u/code/api/", open, 0, "", false},               // the same, with a slash
		{"/home/u/code/new", open, 0, "/home/u/code/new", true}, // projects are open: ask
		{"/home/u/code/new", nil, 2, "/home/u/code/new", true},  // the last session: ask
		{"/home/u/code/new", nil, 0, "", false},                 // nothing to go back to
		{"/home/u", nil, 0, "", true},                           // the home folder (D22)
		{"/home/u/", open, 0, "", true},
	} {
		here, ask := startPick(c.cwd, home, c.open, c.restorable)
		if here != c.here || ask != c.ask {
			t.Errorf("%s (open %v, %d to restore): %q %v, want %q %v", c.cwd, c.open, c.restorable, here, ask, c.here, c.ask)
		}
	}
}
