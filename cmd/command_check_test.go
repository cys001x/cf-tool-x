package cmd

import (
	"strings"
	"testing"
)

func TestValidateCommandLine(t *testing.T) {
	cases := []struct {
		argv []string
		want string
	}{
		{[]string{}, ""},
		{[]string{"--help"}, ""},
		{[]string{"-h"}, ""},
		{[]string{"submit", "1136a"}, ""},
		{[]string{"db", "sync"}, ""},
		{[]string{"timer", "start", "1136a"}, ""},
		{[]string{"timer", "s", "1136a"}, ""},
		{[]string{"timer", "end", "1136a"}, ""},
		{[]string{"timer", "e", "1136a"}, ""},
		{[]string{"timer", "Start", "1136a"}, ""},
		{[]string{"countTime", "start", "1136a"}, ""},
		{[]string{"timer", "pause"}, `Unknown command: "pause"`},
		{[]string{"timer", "p"}, `Unknown command: "p"`},
		{[]string{"timer", "q"}, `Unknown command: "q"`},
		{[]string{"timer", "exit"}, `Unknown command: "exit"`},
		{[]string{"timer", "status"}, `Unknown command: "status"`},
		{[]string{"timer"}, "A timer command is required: start or end."},
		{[]string{"wwww"}, `Unknown command: "wwww"`},
	}
	for _, c := range cases {
		got := ValidateCommandLine(c.argv)
		if c.want == "" {
			if got != "" {
				t.Errorf("ValidateCommandLine(%v) = %q, want no error", c.argv, got)
			}
			continue
		}
		if !strings.Contains(got, c.want) {
			t.Errorf("ValidateCommandLine(%v) = %q, want %q", c.argv, got, c.want)
		}
		if !strings.Contains(got, "cf --help") {
			t.Errorf("ValidateCommandLine(%v) should point to `cf --help`: %q", c.argv, got)
		}
	}
}
