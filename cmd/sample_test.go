package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetSampleIDPrefixed(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"2289a_in1.txt", "2289a_ans1.txt",
		"2289a_in2.txt", "2289a_ans2.txt",
		"2289b_in1.txt", "2289b_ans1.txt",
		"in3.txt", "ans3.txt",
		"in4.txt",
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	samples := getSampleID(dir, "2289a")
	if len(samples) != 2 {
		t.Fatalf("samples = %v, want the two 2289a samples", samples)
	}
	for i, sample := range samples {
		if sample.prefix != "2289a" || sample.index != string(rune('1'+i)) {
			t.Errorf("sample %v = %+v", i, sample)
		}
	}

	all := getSampleID(dir, "")
	if len(all) != 4 {
		t.Fatalf("all samples = %v, want 4 pairs", all)
	}

	legacy := getSampleID(dir, "2289c")
	if len(legacy) != 4 {
		t.Fatalf("unknown prefix should keep every sample, got %v", legacy)
	}
	if legacy[0].inName() != "2289a_in1.txt" {
		t.Errorf("inName = %q", legacy[0].inName())
	}
}
