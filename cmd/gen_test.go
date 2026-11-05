package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenCreatesAnalysisFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := gen("int main() {}", dir, ".cpp"); err != nil {
		t.Fatal(err)
	}
	code, err := os.ReadFile(filepath.Join(dir, "a.cpp"))
	if err != nil {
		t.Fatal(err)
	}
	if string(code) != "int main() {}" {
		t.Fatalf("code = %q", code)
	}
	info, err := os.Stat(filepath.Join(dir, "analysis.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("analysis.md size = %v, want 0", info.Size())
	}

	const notes = "my notes\n"
	if err = os.WriteFile(filepath.Join(dir, "analysis.md"), []byte(notes), 0644); err != nil {
		t.Fatal(err)
	}
	if err = gen("int main() { return 0; }", dir, ".cpp"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "analysis.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != notes {
		t.Fatalf("analysis.md was overwritten: %q", data)
	}
}
