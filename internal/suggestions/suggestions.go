// Package suggestions proposes corrections for likely typos in the domain of
// an email address, such as "gmial.com" -> "gmail.com".
//
// Suggestions only ever point at a domain from a curated list, so a typo can
// never be "corrected" to an arbitrary or attacker-chosen domain. A
// suggestion is returned only when exactly one curated domain is within the
// distance threshold; ties produce no suggestion. The local part is never
// changed and nothing is rewritten automatically.
package suggestions

import "strings"

// DefaultDomains are widely used mailbox providers.
var DefaultDomains = []string{
	"gmail.com", "googlemail.com", "yahoo.com", "yahoo.co.uk", "yahoo.fr", "ymail.com",
	"rocketmail.com", "hotmail.com", "hotmail.co.uk", "hotmail.fr", "outlook.com",
	"live.com", "msn.com", "icloud.com", "me.com", "mac.com", "aol.com", "proton.me",
	"protonmail.com", "gmx.com", "gmx.de", "gmx.net", "web.de", "mail.com", "mail.ru",
	"yandex.ru", "yandex.com", "zoho.com", "fastmail.com", "hey.com", "tutanota.com",
	"comcast.net", "verizon.net", "att.net", "sbcglobal.net", "bellsouth.net", "cox.net",
	"btinternet.com", "orange.fr", "free.fr", "laposte.net", "libero.it", "qq.com",
	"163.com", "126.com", "naver.com", "seznam.cz", "wp.pl", "t-online.de",
}

// Suggester finds the closest curated domain. It is safe for concurrent use.
type Suggester struct {
	domains map[string]struct{}
	list    []string
}

// New returns a Suggester over the given lowercase ASCII domains.
func New(domains []string) *Suggester {
	s := &Suggester{domains: make(map[string]struct{}, len(domains))}
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		if _, dup := s.domains[d]; !dup {
			s.domains[d] = struct{}{}
			s.list = append(s.list, d)
		}
	}
	return s
}

// Suggest returns a curated domain that domain is probably a typo of, or ""
// when there is no confident suggestion.
func (s *Suggester) Suggest(domain string) string {
	if _, exact := s.domains[domain]; exact {
		return ""
	}
	threshold := 1
	if len(domain) >= 12 {
		threshold = 2
	}
	best, bestDist, ties := "", threshold+1, 0
	for _, cand := range s.list {
		if abs(len(cand)-len(domain)) > threshold {
			continue
		}
		d := distance(domain, cand, threshold)
		switch {
		case d < bestDist:
			best, bestDist, ties = cand, d, 0
		case d == bestDist:
			ties++
		}
	}
	if best == "" || ties > 0 {
		return ""
	}
	return best
}

// distance is the optimal string alignment (restricted Damerau-Levenshtein)
// distance between a and b, where a transposition of adjacent characters
// counts as one edit. It returns limit+1 once the distance exceeds limit.
func distance(a, b string, limit int) int {
	prev2 := make([]int, len(b)+1)
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			v := min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				v = min(v, prev2[j-2]+1)
			}
			cur[j] = v
			rowMin = min(rowMin, v)
		}
		if rowMin > limit {
			return limit + 1
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(b)]
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
