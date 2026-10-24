package diff

import "unicode"

// Span is a stretch of a line, in runes: [Start, End).
type Span struct{ Start, End int }

const maxWordDiff = 2000 // longer lines only get their differing middle marked

// WordDiff marks which words of old and new differ. A word is a run of letters, digits and
// underscores; every other character, and every CJK character, is a word of its own.
func WordDiff(old, new string) (oldSpans, newSpans []Span) {
	a, b := []rune(old), []rune(new)
	ta, tb := tokens(a), tokens(b)
	if len(a) > maxWordDiff || len(b) > maxWordDiff || len(ta)*len(tb) > 250_000 {
		p := 0
		for p < len(a) && p < len(b) && a[p] == b[p] {
			p++
		}
		s := 0
		for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
			s++
		}
		if len(a)-s > p {
			oldSpans = []Span{{p, len(a) - s}}
		}
		if len(b)-s > p {
			newSpans = []Span{{p, len(b) - s}}
		}
		return
	}
	inA, inB := lcsMarks(ta, tb, a, b)
	return spansOf(ta, inA), spansOf(tb, inB)
}

type token struct{ start, end int }

func tokens(r []rune) []token {
	var out []token
	for i := 0; i < len(r); {
		j := i + 1
		if wordy(r[i]) {
			for j < len(r) && wordy(r[j]) {
				j++
			}
		}
		out = append(out, token{i, j})
		i = j
	}
	return out
}

func wordy(c rune) bool {
	if unicode.In(c, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
		return false
	}
	return c == '_' || unicode.IsLetter(c) || unicode.IsDigit(c)
}

// lcsMarks reports which tokens of a and b are part of their longest common subsequence.
func lcsMarks(ta, tb []token, a, b []rune) (inA, inB []bool) {
	same := func(i, j int) bool {
		x, y := a[ta[i].start:ta[i].end], b[tb[j].start:tb[j].end]
		if len(x) != len(y) {
			return false
		}
		for k := range x {
			if x[k] != y[k] {
				return false
			}
		}
		return true
	}
	n, m := len(ta), len(tb)
	dp := make([][]int32, n+1)
	for i := range dp {
		dp[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if same(i, j) {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	inA, inB = make([]bool, n), make([]bool, m)
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case same(i, j):
			inA[i], inB[j] = true, true
			i, j = i+1, j+1
		case dp[i+1][j] >= dp[i][j+1]:
			i++
		default:
			j++
		}
	}
	return
}

func spansOf(t []token, common []bool) []Span {
	var out []Span
	for i, tk := range t {
		if common[i] {
			continue
		}
		if n := len(out); n > 0 && out[n-1].End == tk.start {
			out[n-1].End = tk.end
		} else {
			out = append(out, Span{tk.start, tk.end})
		}
	}
	return out
}
