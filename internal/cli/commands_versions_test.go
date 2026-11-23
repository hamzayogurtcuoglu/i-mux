package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// liveLatest (D98): with update_check on it asks for the latest release
// and remembers it; off, or when the answer fails, it says it has none.
func TestLiveLatest(t *testing.T) {
	testutil.Isolate(t)
	asked := 0
	answer, fail := "0.71.3", error(nil)
	old := latestRelease
	latestRelease = func(context.Context) (string, error) { asked++; return answer, fail }
	t.Cleanup(func() { latestRelease = old })

	cfg := config.Default()
	cfg.UpdateCheck = false
	if _, ok := liveLatest(context.Background(), cfg); ok || asked != 0 {
		t.Fatalf("update_check off: ok %v, asked %d", ok, asked)
	}
	cfg.UpdateCheck = true
	if l, ok := liveLatest(context.Background(), cfg); !ok || l != "0.71.3" {
		t.Fatalf("live = %q %v", l, ok)
	}
	if st, _ := state.Read(); st == nil || st.Latest != "0.71.3" {
		t.Errorf("not remembered: %+v", st)
	}
	fail = errors.New("offline")
	if _, ok := liveLatest(context.Background(), cfg); ok {
		t.Error("a failed check counted as live")
	}
}
