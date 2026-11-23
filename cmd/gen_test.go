package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenCreatesAnalysisFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "1136", "a")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := gen("int main() {}", dir, ".cpp", "1136a"); err != nil {
		t.Fatal(err)
	}
	code, err := os.ReadFile(filepath.Join(dir, "1136a.cpp"))
	if err != nil {
		t.Fatal(err)
	}
	if string(code) != "int main() {}" {
		t.Fatalf("code = %q", code)
	}
	info, err := os.Stat(filepath.Join(dir, "1136a_analysis.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("1136a_analysis.md size = %v, want 0", info.Size())
	}

	const notes = "my notes\n"
	if err = os.WriteFile(filepath.Join(dir, "1136a_analysis.md"), []byte(notes), 0644); err != nil {
		t.Fatal(err)
	}
	if err = gen("int main() { return 0; }", dir, ".cpp", "1136a"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "1136a_analysis.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != notes {
		t.Fatalf("1136a_analysis.md was overwritten: %q", data)
	}
	if _, err = os.Stat(filepath.Join(dir, "1136a_1.cpp")); err != nil {
		t.Fatalf("existing code should be kept as 1136a_1.cpp: %v", err)
	}
}

func TestProblemPrefixFromPath(t *testing.T) {
	tests := map[string]string{
		"/home/u/cf/practice/drill/1136a":      "1136a",
		"/home/u/cf/practice/drill/100001a":    "100001a",
		"/home/u/cf/practice/drill_easy/2289a": "2289a",
		"/home/u/cf/contest/1136/a":            "",
		"/home/u/code/workspace":               "",
	}
	for dir, want := range tests {
		if got := problemPrefixFromPath(dir); got != want {
			t.Errorf("problemPrefixFromPath(%q) = %q, want %q", dir, got, want)
		}
	}
}
