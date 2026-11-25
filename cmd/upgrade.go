package cmd

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/cys001x/cf-tool-x/util"
	"github.com/fatih/color"
)

func less(a, b string) bool {
	reg := regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)
	x := reg.FindSubmatch([]byte(a))
	y := reg.FindSubmatch([]byte(b))
	num := func(s []byte) int {
		n, _ := strconv.Atoi(string(s))
		return n
	}
	for i := 1; i <= 3; i++ {
		if num(x[i]) < num(y[i]) {
			return true
		} else if num(x[i]) > num(y[i]) {
			return false
		}
	}
	return false
}

// upgradeAssetCandidates lists the release assets to try, newest naming first.
// Releases ship plain binaries named cf_<tag>_<goos>_<goarch>[.exe]; older
// releases used cf_<tag>_<goos>_<32|64>.zip.
func upgradeAssetCandidates(version, goos, goarch string) []string {
	name := fmt.Sprintf("cf_%v_%v_%v", version, goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	names := []string{name}
	switch goarch {
	case "386":
		names = append(names, fmt.Sprintf("cf_%v_%v_32.zip", version, goos))
	case "amd64":
		names = append(names, fmt.Sprintf("cf_%v_%v_64.zip", version, goos))
	}
	return names
}

func getLatest() (version, note, ptime, url string, size uint, err error) {
	goos := ""
	switch runtime.GOOS {
	case "darwin":
		goos = "darwin"
	case "linux":
		goos = "linux"
	case "windows":
		goos = "windows"
	default:
		err = fmt.Errorf("Not support %v", runtime.GOOS)
		return
	}

	switch runtime.GOARCH {
	case "386", "amd64", "arm64":
	default:
		err = fmt.Errorf("Not support %v", runtime.GOARCH)
		return
	}

	resp, err := http.Get("https://api.github.com/repos/cys001x/cf-tool-x/releases/latest")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	result := make(map[string]interface{})
	if err = json.Unmarshal(body, &result); err != nil {
		return
	}
	version, _ = result["tag_name"].(string)
	if version == "" {
		err = fmt.Errorf("Cannot read the latest release from GitHub")
		return
	}
	note, _ = result["body"].(string)
	if published, ok := result["published_at"].(string); ok {
		if tm, e := time.Parse("2006-01-02T15:04:05Z", published); e == nil {
			ptime = tm.In(time.Local).Format("2006-01-02 15:04")
		}
	}
	assets, _ := result["assets"].([]interface{})
	byName := make(map[string]map[string]interface{}, len(assets))
	for _, tmp := range assets {
		asset, ok := tmp.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := asset["name"].(string); ok {
			byName[name] = asset
		}
	}
	for _, name := range upgradeAssetCandidates(version, goos, runtime.GOARCH) {
		asset, ok := byName[name]
		if !ok {
			continue
		}
		url, _ = asset["browser_download_url"].(string)
		if s, ok := asset["size"].(float64); ok {
			size = uint(s)
		}
		break
	}
	if url == "" {
		err = fmt.Errorf("Release %v has no asset for %v/%v", version, goos, runtime.GOARCH)
	}
	return
}

// WriteCounter progress counter
type WriteCounter struct {
	Count, Total uint
	last         uint
	printedAt    time.Time
}

// Print print progress
func (w *WriteCounter) Print() {
	now := time.Now()
	fmt.Printf("\rProgress: %v/%v KB", w.Count/1024, w.Total/1024)
	if elapsed := now.Sub(w.printedAt).Seconds(); !w.printedAt.IsZero() && elapsed > 0 {
		speed := float64(w.Count-w.last) / elapsed
		fmt.Printf("  Speed: %.0f KB/s", speed/1024)
		if speed > 0 && w.Count <= w.Total {
			fmt.Printf("  Remain: %.0f s", float64(w.Total-w.Count)/speed)
		}
	}
	fmt.Print("           ")
	w.last = w.Count
	w.printedAt = now
}

func (w *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	w.Count += uint(n)
	if time.Since(w.printedAt) >= time.Second {
		w.Print()
	}
	return n, nil
}

func upgrade(url, exePath, version string, size uint) error {
	color.Cyan("Download %v", url)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download release asset: %s", resp.Status)
	}

	counter := &WriteCounter{Total: size}
	counter.Print()
	data, err := io.ReadAll(io.TeeReader(resp.Body, counter))
	counter.Print()
	fmt.Println()
	if err != nil {
		return err
	}
	if size != 0 && uint(len(data)) != size {
		return fmt.Errorf("downloaded %d bytes, expected %d", len(data), size)
	}
	// Releases ship plain binaries; older ones shipped a zip holding the
	// binary as its first file. Accept both.
	payload := data
	if reader, zerr := zip.NewReader(bytes.NewReader(data), int64(len(data))); zerr == nil && len(reader.File) > 0 {
		rc, oerr := reader.File[0].Open()
		if oerr != nil {
			return oerr
		}
		payload, err = ioutil.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
	}

	staged, err := os.CreateTemp(filepath.Dir(exePath), ".cf-update-*"+filepath.Ext(exePath))
	if err != nil {
		return err
	}
	newPath := staged.Name()
	defer os.Remove(newPath)
	color.Cyan("Save the new one to %v", newPath)
	_, writeErr := staged.Write(payload)
	closeErr := staged.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Chmod(newPath, 0755); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, newPath, "--version").Output()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("verify downloaded executable: version check timed out")
	}
	if err != nil {
		return fmt.Errorf("verify downloaded executable: %w", err)
	}
	if got, want := strings.TrimSpace(string(output)), "Codeforces Tool (cf) "+version; got != want {
		return fmt.Errorf("verify downloaded executable: got %q, expected %q", got, want)
	}

	oldPath := newPath + ".old"
	color.Cyan("Move the old one to %v", oldPath)
	if err := os.Rename(exePath, oldPath); err != nil {
		return err
	}
	if err := os.Rename(newPath, exePath); err != nil {
		if restoreErr := os.Rename(oldPath, exePath); restoreErr != nil {
			return fmt.Errorf("install update: %w; restore failed, previous executable at %s: %w", err, oldPath, restoreErr)
		}
		return fmt.Errorf("install update (previous executable restored): %w", err)
	}
	if err := os.Remove(oldPath); err != nil {
		color.Yellow("Previous executable kept at %v: %v", oldPath, err)
	}
	return nil
}

// Upgrade itself
func Upgrade() (err error) {
	color.Cyan("Checking version")
	latest, note, ptime, url, size, err := getLatest()
	if err != nil {
		return
	}
	version := Args.Version
	if !less(version, latest) {
		color.Green("Current version %v is the latest", version)
		return
	}

	color.Red("Current version is %v", version)
	color.Green("The latest version is %v, published at %v", latest, ptime)
	fmt.Println(note)

	if !util.YesOrNo("Do you want to upgrade (y/n)? ") {
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		return
	}

	if exePath, err = filepath.EvalSymlinks(exePath); err != nil {
		return
	}

	if err = upgrade(url, exePath, latest, size); err != nil {
		return
	}

	color.Green("Successfully updated to version %v", latest)
	return
}
