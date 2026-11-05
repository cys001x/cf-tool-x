package client

import "testing"

func TestParseContestDiv(t *testing.T) {
	cases := map[string]string{
		"Codeforces Round 546 (Div. 2)":           "2",
		"Codeforces Round 1000 (Div. 1 + Div. 2)": "1+2",
		"Codeforces Round 1 (Div.1)":              "1",
		"Educational Codeforces Round 100":        "",
		"Codeforces Round 3 (Div. 3)":             "3",
	}
	for name, want := range cases {
		if got := parseContestDiv(name); got != want {
			t.Errorf("parseContestDiv(%q) = %q, want %q", name, got, want)
		}
	}
}
