package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLanguageExt(t *testing.T) {
	tests := map[string]string{
		"GNU G++17 7.3.0":          "cpp",
		"C++23 (GCC 14-64, msys2)": "cpp",
		"GNU C11 5.1.0":            "c",
		"Java 11.0.5":              "java",
		"JavaScript V8 4.8.0":      "js",
		"Node.js 9.4.0":            "js",
		"Python 3.7.2":             "py",
		"PyPy 3.6 (7.2.0)":         "py",
		"Rust 1.35.0":              "rs",
		"Go 1.12.6":                "go",
		"Kotlin 1.3.10":            "kt",
		"Mono C# 5.18":             "cs",
		"Delphi 7":                 "pas",
	}
	for lang, want := range tests {
		got, ok := LanguageExt(lang)
		if !ok || got != want {
			t.Errorf("LanguageExt(%q) = %q, %v; want %q, true", lang, got, ok, want)
		}
	}
	if _, ok := LanguageExt("Unknown Language XYZ"); ok {
		t.Error("LanguageExt(\"Unknown Language XYZ\") should not match any extension")
	}
}

func TestFetchSubmissionCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/submission":
			w.Write([]byte(`<html><body><pre>int main() { return 0; }</pre></body></html>`))
		case "/challenge":
			w.Write([]byte(`<html><title>Just a moment...</title></html>`))
		case "/error":
			w.Write([]byte(`<html><script>Codeforces.showMessage("Denied"); Codeforces.reformatTimes();</script></html>`))
		}
	}))
	defer server.Close()

	c := &Client{client: server.Client()}

	code, err := c.FetchSubmissionCode(server.URL + "/submission")
	if err != nil {
		t.Fatalf("FetchSubmissionCode: %v", err)
	}
	if code != "int main() { return 0; }" {
		t.Errorf("unexpected code %q", code)
	}

	if _, err = c.FetchSubmissionCode(server.URL + "/challenge"); err == nil {
		t.Error("expected an error for a Cloudflare challenge page")
	}

	if _, err = c.FetchSubmissionCode(server.URL + "/error"); err == nil || err.Error() != "Denied" {
		t.Errorf("expected the page message, got %v", err)
	}
}
