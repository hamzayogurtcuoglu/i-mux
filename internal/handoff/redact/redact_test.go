package redact

import (
	"strings"
	"testing"
)

// Secrets are assembled at runtime so that no real-looking secret appears
// verbatim in the repository (push protection, fixture guard).
func rep(s string, n int) string { return strings.Repeat(s, n) }

func TestPositive(t *testing.T) {
	cases := []struct{ in, kind string }{
		{"-----BEGIN " + "RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA" + rep("x", 64) + "\n-----END RSA PRIVATE KEY-----", "private-key"},
		{"-----BEGIN " + "OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXk=\n-----END OPENSSH PRIVATE KEY-----", "private-key"},
		{"key=" + "AKIA" + "IOSFODNN7EXAMPLE", "aws-access-key"},
		{"token " + "gh" + "p_" + rep("A1b2", 9), "github-token"},
		{"gh" + "s_" + rep("Z9", 18), "github-token"},
		{"github" + "_pat_" + rep("11ABCDEFG", 4), "github-token"},
		{"ANTHROPIC_API_KEY=" + "sk-" + "ant-api03-" + rep("aB3_", 20), "anthropic-key"},
		{"OPENAI_API_KEY=" + "sk-" + "proj-" + rep("Ab3d", 12), "openai-key"},
		{"sk-" + rep("Xy7", 16), "openai-key"},
		{"key: " + "AI" + "za" + rep("S", 10) + rep("y1_-", 6) + "a", "google-api-key"},
		{"xo" + "xb-" + "123456789012-abcdefghij", "slack-token"},
		{"curl -H 'Authorization: " + "Bearer " + "eyJhbGciOi" + rep("J9.x", 6) + "'", "bearer-token"},
	}
	for _, c := range cases {
		got, n := String(c.in)
		if n != 1 || !strings.Contains(got, "[REDACTED:"+c.kind+"]") {
			t.Errorf("%s: got %q (n=%d)", c.kind, got, n)
		}
	}
	got, _ := String("Authorization: " + "Bearer " + rep("t0k", 8))
	if got != "Authorization: Bearer [REDACTED:bearer-token]" {
		t.Fatalf("header text should stay: %q", got)
	}
}

func TestNegative(t *testing.T) {
	for _, s := range []string{
		"skip the next step",
		"var sk_test_var = 1",
		"the AKIAlike prefix appears in prose",
		"AKIA1234",
		"use sk-learn for this",
		"class=\"sk-some-long-kebab-case-identifier-name-here\"",
		"ghp_short",
		"Authorization: Bearer <token>",
		"func AIzaLike() {}",
		"-----BEGIN PUBLIC KEY-----\nabc\n-----END PUBLIC KEY-----",
		"ğüşıöç 🎉 normal text",
	} {
		got, n := String(s)
		if n != 0 || got != s {
			t.Errorf("%q mangled into %q", s, got)
		}
	}
}

func FuzzString(f *testing.F) {
	f.Add("Authorization: Bearer abcdefghijklmnopqrstuvwxyz")
	f.Add("-----BEGIN PRIVATE KEY-----")
	f.Fuzz(func(t *testing.T, s string) {
		out, n := String(s)
		if n == 0 && out != s {
			t.Fatal("changed without counting")
		}
	})
}
