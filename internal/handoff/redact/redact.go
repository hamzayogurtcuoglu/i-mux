// Package redact removes high-confidence secrets from handoff documents
// (§8.5). It is best effort and deliberately narrow so that ordinary code is
// never mangled. Matches become "[REDACTED:<kind>]".
package redact

import (
	"regexp"
	"strings"
)

type rule struct {
	kind string
	re   *regexp.Regexp
	// group is the submatch index to replace (0 = whole match).
	group int
	// check optionally rejects a match.
	check func(string) bool
}

func hasDigit(s string) bool { return strings.ContainsAny(s, "0123456789") }

var rules = []rule{
	{kind: "private-key", re: regexp.MustCompile(`-----BEGIN ((?:RSA |EC |DSA |OPENSSH |ENCRYPTED |PGP )?PRIVATE KEY(?: BLOCK)?)-----[\s\S]*?(?:-----END ([A-Z ]*PRIVATE KEY(?: BLOCK)?)-----|$)`)},
	{kind: "aws-access-key", re: regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)},
	{kind: "github-token", re: regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,255}\b`)},
	{kind: "github-token", re: regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,255}\b`)},
	{kind: "anthropic-key", re: regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}`)},
	{kind: "openai-key", re: regexp.MustCompile(`\bsk-(?:proj-|svcacct-|admin-)?[A-Za-z0-9_-]{32,}`), check: hasDigit},
	{kind: "google-api-key", re: regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`)},
	{kind: "slack-token", re: regexp.MustCompile(`\bxox[abpr]-[A-Za-z0-9-]{10,}`)},
	{kind: "bearer-token", re: regexp.MustCompile(`(?i)\bauthorization:\s*bearer\s+([A-Za-z0-9._~+/=-]{16,})`), group: 1},
}

// String redacts s and returns the result and the number of replacements.
func String(s string) (string, int) {
	n := 0
	for _, r := range rules {
		if r.group == 0 {
			s = r.re.ReplaceAllStringFunc(s, func(m string) string {
				if r.check != nil && !r.check(m) {
					return m
				}
				n++
				return "[REDACTED:" + r.kind + "]"
			})
			continue
		}
		s = r.re.ReplaceAllStringFunc(s, func(m string) string {
			sub := r.re.FindStringSubmatchIndex(m)
			if len(sub) < 2*(r.group+1) || sub[2*r.group] < 0 {
				return m
			}
			n++
			return m[:sub[2*r.group]] + "[REDACTED:" + r.kind + "]" + m[sub[2*r.group+1]:]
		})
	}
	return s, n
}
