package client

import "testing"

func TestFilePrefix(t *testing.T) {
	tests := []struct {
		info Info
		pid  string
		want string
	}{
		{Info{ProblemType: "contest", ContestID: "2289"}, "A", "2289a"},
		{Info{ProblemType: "gym", ContestID: "100001"}, "b1", "100001b1"},
		{Info{ProblemType: "group", ContestID: "269760"}, "c", "269760c"},
		{Info{ProblemType: "acmsguru", ContestID: "99999"}, "100", "100"},
		{Info{ProblemType: "contest", ContestID: "2289"}, "", "2289"},
		{Info{ProblemType: "contest"}, "a", "a"},
	}
	for _, test := range tests {
		if got := test.info.FilePrefix(test.pid); got != test.want {
			t.Errorf("FilePrefix(%q) = %q, want %q", test.pid, got, test.want)
		}
	}
}

func TestPrefixedName(t *testing.T) {
	if got := PrefixedName("2289a", "statement.md"); got != "2289a_statement.md" {
		t.Errorf("PrefixedName = %q", got)
	}
	if got := PrefixedName("", "analysis.md"); got != "analysis.md" {
		t.Errorf("PrefixedName without prefix = %q", got)
	}
}
