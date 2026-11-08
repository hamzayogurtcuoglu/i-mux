// Package gotofile is the editor's Quick Open in a popup (§9.3 "Go to File",
// Ctrl+P): type a few letters of a file's name, Enter opens it.
package gotofile

import (
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Query is a parsed query: its parts (all must match), whether it is case
// sensitive (it has a capital), and a line from a trailing ":42".
type Query struct {
	Parts []string
	Exact bool
	Line  int
}

// Parse splits a typed query.
func Parse(q string) Query {
	var out Query
	q = strings.TrimSpace(q)
	if i := strings.LastIndex(q, ":"); i >= 0 {
		if n, err := strconv.Atoi(q[i+1:]); err == nil && n > 0 {
			out.Line = n
			q = q[:i]
		} else if i == len(q)-1 {
			q = q[:i] // "name:" while the line is being typed
		}
	}
	for _, r := range q {
		if unicode.IsUpper(r) {
			out.Exact = true
			break
		}
	}
	for _, p := range strings.Fields(q) {
		if !out.Exact {
			p = strings.ToLower(p)
		}
		out.Parts = append(out.Parts, p)
	}
	return out
}

// Empty reports a query with nothing to match.
func (q Query) Empty() bool { return len(q.Parts) == 0 }

// Scores of a match (higher is better).
const (
	scoreChar     = 1
	bonusStart    = 8  // at a word start: after / . _ - or a space, a capital after a small letter
	bonusConsec   = 14 // right after the previous matched letter: typed together, found together
	bonusBase     = 4  // in the file's name, not its folders
	bonusFirst    = 2  // the file name's first letter
	penaltyGap    = 1  // per letter skipped between two matched ones
	penaltyGapMax = 6
	none          = -1 << 30
)

// Match scores path for q: ok when every part's letters appear in order;
// pos are the matched rune indices (sorted) for highlighting.
func Match(path string, q Query) (score int, pos []int, ok bool) {
	rs := []rune(path)
	low := rs
	if !q.Exact {
		low = []rune(strings.ToLower(path))
	}
	base := strings.LastIndex(path, "/") + 1
	base = len([]rune(path[:base]))
	seen := map[int]bool{}
	for _, part := range q.Parts {
		s, p, good := matchPart(rs, low, []rune(part), base)
		if !good {
			return 0, nil, false
		}
		score += s
		for _, i := range p {
			seen[i] = true
		}
	}
	for i := range seen {
		pos = append(pos, i)
	}
	sort.Ints(pos)
	return score, pos, true
}

// matchPart finds the best placement of part's letters in low (a dynamic
// program over letter × position, linear in both with a running best for
// gaps) and returns its score and positions.
func matchPart(orig, low, part []rune, base int) (int, []int, bool) {
	n, m := len(low), len(part)
	if m == 0 {
		return 0, nil, true
	}
	if m > n || !subsequence(low, part) {
		return 0, nil, false // most files: no placement to score
	}
	bonus := make([]int, n)
	for j := range low {
		b := scoreChar
		if j == 0 || strings.ContainsRune("/._- ", low[j-1]) ||
			unicode.IsUpper(orig[j]) && unicode.IsLower(orig[j-1]) {
			b += bonusStart
		}
		if j >= base {
			b += bonusBase
		}
		if j == base {
			b += bonusFirst
		}
		bonus[j] = b
	}
	// best[i][j]: the best score of part[:i+1] with part[i] at j; from[i][j]
	// the position of part[i-1].
	best := make([][]int, m)
	from := make([][]int, m)
	for i := range best {
		best[i] = make([]int, n)
		from[i] = make([]int, n)
	}
	for j := 0; j < n; j++ {
		best[0][j], from[0][j] = none, -1
		if low[j] == part[0] {
			best[0][j] = bonus[j]
		}
	}
	far := penaltyGapMax/penaltyGap + 1 // from this many letters back the gap costs the most
	for i := 1; i < m; i++ {
		prev := best[i-1]
		run, runAt := none, -1 // the best prev[k] for k <= j-1-far (the gap is capped)
		for j := 0; j < n; j++ {
			best[i][j], from[i][j] = none, -1
			if k := j - 1 - far; k >= 0 && prev[k] > run {
				run, runAt = prev[k], k
			}
			if low[j] != part[i] {
				continue
			}
			if j >= 1 && prev[j-1] > none {
				best[i][j], from[i][j] = prev[j-1]+bonus[j]+bonusConsec, j-1
			}
			for k := max(j-far, 0); k <= j-2; k++ { // a short gap
				if prev[k] > none {
					if c := prev[k] + bonus[j] - (j-k-1)*penaltyGap; c > best[i][j] {
						best[i][j], from[i][j] = c, k
					}
				}
			}
			if runAt >= 0 {
				if c := run + bonus[j] - penaltyGapMax; c > best[i][j] {
					best[i][j], from[i][j] = c, runAt
				}
			}
		}
	}
	end, top := -1, none
	for j := 0; j < n; j++ {
		if best[m-1][j] > top {
			top, end = best[m-1][j], j
		}
	}
	if end < 0 {
		return 0, nil, false
	}
	pos := make([]int, m)
	for i, j := m-1, end; i >= 0; i-- {
		pos[i] = j
		j = from[i][j]
	}
	return top, pos, true
}

// Result is one file of the list.
type Result struct {
	Path  string
	Score int
	Pos   []int
}

// Rank matches files against q, best first: score, then shorter paths,
// then by name; at most limit.
func Rank(files []string, q Query, limit int) []Result {
	var out []Result
	for _, f := range files {
		if s, p, ok := Match(f, q); ok {
			out = append(out, Result{Path: f, Score: s, Pos: p})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if len(out[i].Path) != len(out[j].Path) {
			return len(out[i].Path) < len(out[j].Path)
		}
		return out[i].Path < out[j].Path
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// subsequence reports whether part's letters appear in s in order.
func subsequence(s, part []rune) bool {
	i := 0
	for _, r := range s {
		if i < len(part) && r == part[i] {
			i++
		}
	}
	return i == len(part)
}
