package syntax

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

type sharedCase struct {
	Email  string  `json:"email"`
	Reason *string `json:"reason"`
	Domain string  `json:"domain"`
}

func loadSharedCases(t testing.TB) []sharedCase {
	t.Helper()
	b, err := os.ReadFile("../../testdata/syntax-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Cases []sharedCase `json:"cases"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f.Cases
}

func TestSharedCases(t *testing.T) {
	for _, tc := range loadSharedCases(t) {
		t.Run(tc.Email, func(t *testing.T) {
			got := Parse(tc.Email)
			wantReason := ""
			if tc.Reason != nil {
				wantReason = *tc.Reason
			}
			if string(got.Reason) != wantReason {
				t.Fatalf("Parse(%q).Reason = %q, want %q", tc.Email, got.Reason, wantReason)
			}
			if got.Valid != (tc.Reason == nil) {
				t.Fatalf("Parse(%q).Valid = %v", tc.Email, got.Valid)
			}
			if got.Domain != tc.Domain {
				t.Fatalf("Parse(%q).Domain = %q, want %q", tc.Email, got.Domain, tc.Domain)
			}
		})
	}
}

func TestLengthBoundaries(t *testing.T) {
	label := func(n int) string { return strings.Repeat("a", n) }

	// Build a domain of exactly n octets from 63-octet labels and a ".com" tail.
	domainOf := func(n int) string {
		var parts []string
		remaining := n - len(".com")
		for remaining > 0 {
			l := min(MaxLabelLength, remaining)
			if remaining-l == 1 { // avoid leaving a lone dot with no label
				l--
			}
			parts = append(parts, label(l))
			remaining -= l + 1
		}
		d := strings.Join(parts, ".") + ".com"
		if len(d) != n {
			t.Fatalf("domainOf(%d) built %d octets", n, len(d))
		}
		return d
	}

	tests := []struct {
		name   string
		email  string
		reason Reason
	}{
		{"local at limit", label(64) + "@example.com", ""},
		{"local over limit", label(65) + "@example.com", ReasonLocalTooLong},
		{"label at limit", "a@" + label(63) + ".com", ""},
		{"label over limit", "a@" + label(64) + ".com", ReasonDomainLabelTooLong},
		{"address at limit", label(64) + "@" + domainOf(254-65), ""},
		{"address over limit", label(64) + "@" + domainOf(254-64), ReasonTooLong},
		{"domain at limit", "a@" + domainOf(253), ReasonTooLong}, // 1+1+253 > 254
		{"domain over limit", "a@" + domainOf(254), ReasonDomainTooLong},
		{"raw input cap", label(600) + "@example.com", ReasonTooLong},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse(tc.email).Reason; got != tc.reason {
				t.Fatalf("reason = %q, want %q (len %d)", got, tc.reason, len(tc.email))
			}
		})
	}
}

func TestIDNANormalization(t *testing.T) {
	tests := []struct {
		in, want string
		reason   Reason
	}{
		{"bücher.example", "xn--bcher-kva.example", ""},
		{"BÜCHER.EXAMPLE", "xn--bcher-kva.example", ""},
		{"例え.jp", "xn--r8jz45g.jp", ""},
		{"xn--bcher-kva.example", "xn--bcher-kva.example", ""},
		// Fullwidth characters are mapped by UTS #46.
		{"ｅｘａｍｐｌｅ.com", "example.com", ""},
		// Invalid punycode.
		{"xn--a.example", "", ReasonDomainInvalid},
		// Disallowed code point.
		{"exa\u2028mple.com", "", ReasonWhitespace},
		// Unassigned code point.
		{"exa\u0378mple.com", "", ReasonDomainInvalid},
		// Emoji labels are valid under UTS #46 (and exist under some TLDs).
		{"exa😀mple.com", "xn--example-9o25f.com", ""},
	}
	for _, tc := range tests {
		got, reason := NormalizeDomain(tc.in)
		if got != tc.want || reason != tc.reason {
			t.Errorf("NormalizeDomain(%q) = %q, %q; want %q, %q", tc.in, got, reason, tc.want, tc.reason)
		}
	}
}

func TestParseDoesNotModifyLocalPart(t *testing.T) {
	r := Parse("Mixed.Case+Tag@Example.com")
	if !r.Valid || r.Local != "Mixed.Case+Tag" {
		t.Fatalf("got %+v", r)
	}
}

func FuzzParse(f *testing.F) {
	for _, tc := range loadSharedCases(f) {
		f.Add(tc.Email)
	}
	f.Add("\x00@example.com")
	f.Add("a@\xff.com")
	f.Fuzz(func(t *testing.T, email string) {
		r := Parse(email)
		if r.Valid == (r.Reason != "") {
			t.Fatalf("Valid=%v but Reason=%q", r.Valid, r.Reason)
		}
		if !r.Valid {
			return
		}
		if len(r.Local) == 0 || len(r.Local) > MaxLocalLength {
			t.Fatalf("local length %d", len(r.Local))
		}
		if len(r.Domain) > MaxDomainLength || len(r.Local)+1+len(r.Domain) > MaxAddressLength {
			t.Fatalf("length limits exceeded: %q", email)
		}
		if !utf8.ValidString(email) || r.Domain != strings.ToLower(r.Domain) {
			t.Fatalf("bad normalization for %q: %q", email, r.Domain)
		}
		for i := 0; i < len(r.Domain); i++ {
			if r.Domain[i] >= utf8.RuneSelf {
				t.Fatalf("domain not ASCII: %q", r.Domain)
			}
		}
		// Normalization must be idempotent.
		if again, reason := NormalizeDomain(r.Domain); again != r.Domain || reason != "" {
			t.Fatalf("NormalizeDomain(%q) = %q, %q", r.Domain, again, reason)
		}
	})
}
