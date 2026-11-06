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
		{[]string{"start", "1136a"}, ""},
		{[]string{"start"}, ""},
		{[]string{"end", "1136a"}, ""},
		{[]string{"end"}, ""},
		{[]string{"timer", "start", "1136a"}, `Unknown command: "timer"`},
		{[]string{"countTime", "start", "1136a"}, `Unknown command: "countTime"`},
		{[]string{"pause"}, `Unknown command: "pause"`},
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
