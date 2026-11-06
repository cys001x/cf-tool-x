package cmd

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
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
}

// Print print progress
func (w *WriteCounter) Print() {
	fmt.Printf("\rProgress: %v/%v KB  Speed: %v KB/s  Remain: %.0f s           ",
		w.Count/1024, w.Total/1024, (w.Count-w.last)/1024, float64(w.Total-w.Count)/float64(w.Count-w.last))
	w.last = w.Count
}

func (w *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	w.Count += uint(n)
	return n, nil
}

func upgrade(url, exePath string, size uint) (err error) {
	updateDir := filepath.Dir(exePath)

	oldPath := filepath.Join(updateDir, fmt.Sprintf(".%s.old", filepath.Base(exePath)))
	color.Cyan("Move the old one to %v", oldPath)
	if err = os.Rename(exePath, oldPath); err != nil {
		return
	}
	defer func() {
		if err != nil {
			color.Cyan("Move the old one back")
			if e := os.Rename(oldPath, exePath); e != nil {
				color.Red(e.Error())
			}
		} else {
			color.Cyan("Remove the old one")
			if e := os.Remove(oldPath); e != nil {
				color.Red(e.Error() + "\nYou could remove it manually")
			}
		}
	}()

	color.Cyan("Download %v", url)
	counter := &WriteCounter{Count: 0, Total: size, last: 0}
	counter.Print()

	ticker := time.NewTicker(time.Second)
	go func() {
		for range ticker.C {
			counter.Print()
		}
	}()

	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(io.TeeReader(resp.Body, counter))
	ticker.Stop()
	counter.Print()
	fmt.Println()
	if err != nil {
		return
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

	newPath := filepath.Join(updateDir, fmt.Sprintf(".%s.new", filepath.Base(exePath)))
	color.Cyan("Save the new one to %v", newPath)
	if err = ioutil.WriteFile(newPath, payload, 0755); err != nil {
		return
	}

	if err = os.Rename(newPath, exePath); err != nil {
		color.Cyan("Delete the new one %v", newPath)
		if e := os.Remove(newPath); e != nil {
			color.Red(e.Error())
		}
	}

	return
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

	if err = upgrade(url, exePath, size); err != nil {
		return
	}

	color.Green("Successfully updated to version %v", latest)
	return
}
