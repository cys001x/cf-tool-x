package cmd

import (
	"reflect"
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
