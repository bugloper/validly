package suggestions

import "testing"

func TestSuggest(t *testing.T) {
	s := New(DefaultDomains)
	for in, want := range map[string]string{
		"gmail.xom":      "gmail.com",
		"gmal.com":       "gmail.com",
		"gmial.com":      "gmail.com",
		"hotmial.com":    "hotmail.com",
		"outlok.com":     "outlook.com",
		"yaho.com":       "yahoo.com",
		"icloud.co":      "icloud.com",
		"protonmial.com": "protonmail.com",
		// Exact matches and real providers are left alone.
		"gmail.com":   "",
		"mail.com":    "",
		"gmx.com":     "",
		"example.com": "",
		// Too far from anything curated.
		"gnail.co.uk": "",
		"company.io":  "",
		// One edit from both gmx.net and gmx.de: ambiguous, no suggestion.
		"gmx.ne": "",
		// One edit from both me.com and mac.com: ambiguous, no suggestion.
		"mec.com": "",
	} {
		if got := s.Suggest(in); got != want {
			t.Errorf("Suggest(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSuggestOnlyReturnsCuratedDomains(t *testing.T) {
	s := New([]string{"gmail.com"})
	if got := s.Suggest("gmail.co"); got != "gmail.com" {
		t.Fatalf("got %q", got)
	}
	if got := s.Suggest("hotmial.com"); got != "" {
		t.Fatalf("suggested %q outside the curated list", got)
	}
}

func TestDistance(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"abc", "acb", 1},
		{"abc", "ab", 1},
		{"abc", "xbc", 1},
		{"kitten", "sitting", 3},
	} {
		if got := distance(tc.a, tc.b, 10); got != tc.want {
			t.Errorf("distance(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
	if got := distance("kitten", "sitting", 1); got != 2 {
		t.Errorf("limited distance = %d, want 2", got)
	}
}

func FuzzSuggest(f *testing.F) {
	s := New(DefaultDomains)
	f.Add("gmial.com")
	f.Fuzz(func(t *testing.T, in string) {
		got := s.Suggest(in)
		if got == "" {
			return
		}
		if _, ok := s.domains[got]; !ok {
			t.Fatalf("suggested non-curated domain %q", got)
		}
		if got == in {
			t.Fatalf("suggested the input itself")
		}
	})
}
