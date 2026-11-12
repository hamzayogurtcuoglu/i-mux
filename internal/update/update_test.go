package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.27.0", "0.26.1", true},
		{"0.26.1", "0.26.1", false},
		{"0.26.1", "0.27.0", false},
		{"1.0.0", "0.99.99", true},
		{"v0.10.0", "0.9.9", true},
		{"0.27.0", "dev", false},
		{"x", "0.1.0", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

func TestLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases/latest" {
			http.Redirect(w, r, "/releases/tag/v0.27.0", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusGatewayTimeout)
	}))
	defer srv.Close()
	old := LatestURL
	defer func() { LatestURL = old }()
	LatestURL = srv.URL + "/releases/latest"
	if v, err := Latest(context.Background()); err != nil || v != "0.27.0" {
		t.Fatalf("latest %q %v", v, err)
	}
	LatestURL = srv.URL + "/down"
	if _, err := Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "504") {
		t.Fatalf("an outage: %v", err)
	}
}

func TestInstallCmd(t *testing.T) {
	c := InstallCmd("0.27.0", "/Users/me/.local/bin")
	if c[0] != "sh" || !strings.Contains(c[2], "--version") || c[4] != "https://github.com/"+Repo+"/releases/download/v0.27.0/install.sh" ||
		c[5] != "v0.27.0" || c[6] != "/Users/me/.local/bin" {
		t.Fatalf("%q", c)
	}
	if !Homebrew("/opt/homebrew/Cellar/imux/0.26.1/bin") || Homebrew("/Users/me/.local/bin") {
		t.Fatal("homebrew")
	}
}
