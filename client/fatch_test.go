package client

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestWriteContentDownloadsStatementImages(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nfake-image")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/problem":
			w.Write([]byte(`<html><script>handle = "tester"</script>
<div class="problem-statement">
  <div class="header"></div>
  <div><p>Look <img src="/img/a.png" alt="Figure A"> and <img src="/missing.png" alt="Gone"></p></div>
</div>
</html>`))
		case "/img/a.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write(png)
		}
	}))
	defer server.Close()

	c := &Client{client: server.Client()}
	dir := t.TempDir()
	if err := c.writeContentToMdFile([]byte(`<html><script>handle = "tester"</script>
<div class="problem-statement">
  <div class="header"></div>
  <div><p>Look <img src="/img/a.png" alt="Figure A"> and <img src="/missing.png" alt="Gone"></p></div>
</div>
</html>`), server.URL+"/problem", dir, "statement.md"); err != nil {
		t.Fatalf("writeContentToMdFile: %v", err)
	}

	md, err := os.ReadFile(filepath.Join(dir, "statement.md"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`!\[Figure A\]\(assets/a_[0-9a-f]{8}\.png\)`).Find(md)
	if match == nil {
		t.Fatalf("local image reference missing in:\n%s", md)
	}
	name := regexp.MustCompile(`assets/(a_[0-9a-f]{8}\.png)`).FindSubmatch(md)
	saved, err := os.ReadFile(filepath.Join(dir, "assets", string(name[1])))
	if err != nil {
		t.Fatalf("image not saved: %v", err)
	}
	if string(saved) != string(png) {
		t.Fatalf("saved image mismatch: %q", saved)
	}
	if !regexp.MustCompile(regexp.QuoteMeta("](" + server.URL + "/missing.png)")).Match(md) {
		t.Fatalf("failed image should keep its absolute URL:\n%s", md)
	}
}
