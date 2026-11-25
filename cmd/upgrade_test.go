package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestUpgradeAssetCandidates(t *testing.T) {
	cases := []struct {
		goos, goarch string
		want         []string
	}{
		{"darwin", "amd64", []string{"cf_v1.3.3_darwin_amd64", "cf_v1.3.3_darwin_64.zip"}},
		{"darwin", "arm64", []string{"cf_v1.3.3_darwin_arm64"}},
		{"linux", "amd64", []string{"cf_v1.3.3_linux_amd64", "cf_v1.3.3_linux_64.zip"}},
		{"linux", "arm64", []string{"cf_v1.3.3_linux_arm64"}},
		{"windows", "amd64", []string{"cf_v1.3.3_windows_amd64.exe", "cf_v1.3.3_windows_64.zip"}},
		{"linux", "386", []string{"cf_v1.3.3_linux_386", "cf_v1.3.3_linux_32.zip"}},
	}
	for _, c := range cases {
		got := upgradeAssetCandidates("v1.3.3", c.goos, c.goarch)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("upgradeAssetCandidates(v1.3.3, %q, %q) = %v, want %v", c.goos, c.goarch, got, c.want)
		}
	}
}

func TestUpgradePreservesInstalledExecutableBeforeVerification(t *testing.T) {
	const original = "installed executable"
	const invalid = "not an executable"
	cases := []struct {
		name   string
		status int
		size   uint
		want   string
	}{
		{"HTTP error", http.StatusBadGateway, uint(len(invalid)), "502 Bad Gateway"},
		{"incomplete download", http.StatusOK, uint(len(invalid) + 1), "bytes, expected"},
		{"invalid executable", http.StatusOK, uint(len(invalid)), "verify downloaded executable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			exePath := filepath.Join(dir, "cf.exe")
			if err := os.WriteFile(exePath, []byte(original), 0755); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if data, err := os.ReadFile(exePath); err != nil || string(data) != original {
					t.Errorf("installed file during download = %q, %v", data, err)
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, invalid)
			}))
			defer server.Close()

			if err := upgrade(server.URL, exePath, "v1.3.3", tc.size); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("upgrade error = %v, want %q", err, tc.want)
			}
			if data, err := os.ReadFile(exePath); err != nil || string(data) != original {
				t.Errorf("installed file after failure = %q, %v", data, err)
			}
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 1 {
				t.Errorf("directory after failure = %v, %v", files, err)
			}
		})
	}
}
