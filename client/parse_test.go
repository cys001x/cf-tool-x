package client

import (
	"os"
	"path/filepath"
	"testing"
)

const samplePage = `<html>
<div class="input-file input-standard">standard input</div>
<div class="output-file">standard output</div>
<div class="input"><pre>1 2</pre></div>
<div class="output"><pre>3</pre></div>
</html>`

func TestWriteSamplesUsesPrefix(t *testing.T) {
	dir := t.TempDir()
	n, standard, err := writeSamples([]byte(samplePage), dir, "1136a", nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || !standard {
		t.Fatalf("writeSamples = %v, %v", n, standard)
	}
	for name, want := range map[string]string{
		"1136a_in1.txt":  "1 2\n",
		"1136a_ans1.txt": "3\n",
	} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%v: %v", name, err)
		}
		if string(data) != want {
			t.Errorf("%v = %q, want %q", name, data, want)
		}
	}
}

func TestWriteSamplesLegacyNames(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := writeSamples([]byte(samplePage), dir, "", nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"in1.txt", "ans1.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%v: %v", name, err)
		}
	}
}
