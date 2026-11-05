package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/cys001x/cf-tool-x/client"
	"github.com/cys001x/cf-tool-x/config"
	"github.com/cys001x/cf-tool-x/util"
	"github.com/fatih/color"
)

// findLocalCodes lists the files in dir that look like solution code, using
// the configured template suffixes and the known language extensions.
func findLocalCodes(dir string, cfg *config.Config) (names []string) {
	known := map[string]bool{}
	for lang, ext := range client.LangsExt {
		// The empty key maps to "txt", which is used for samples, not code.
		if lang != "" {
			known[ext] = true
		}
	}
	for _, template := range cfg.Template {
		for _, suffix := range template.Suffix {
			known[strings.TrimPrefix(suffix, ".")] = true
		}
	}

	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(entry.Name()), "."))
		if known[ext] {
			names = append(names, entry.Name())
		}
	}
	return
}

// Sync command
func Sync() (err error) {
	cfg := config.Instance
	cln := client.Instance
	info := Args.Info

	var problems []client.SyncedProblem
	var latest map[string]client.LatestSubmission
	work := func() (e error) {
		problems, latest, e = cln.Sync(info)
		return
	}
	if err = work(); err != nil {
		if err = loginAgain(cln, err); err == nil {
			err = work()
		}
	}
	if err != nil {
		return
	}

	source := ""
	templateExt := ""
	templateLoaded := false
	loadTemplate := func() error {
		if templateLoaded {
			return nil
		}
		if len(cfg.Template) == 0 {
			return errors.New("You have to add at least one code template by `cf config`")
		}
		path := cfg.Template[cfg.Default].Path
		templateExt = filepath.Ext(path)
		s, e := readTemplateSource(path, cln)
		if e != nil {
			return e
		}
		source = s
		templateLoaded = true
		return nil
	}

	for i := range problems {
		problem := &problems[i]
		ref, hasSubmission := latest[strings.ToLower(problem.ID)]
		local := findLocalCodes(problem.Path, cfg)

		switch {
		case hasSubmission && ref.URL == "":
			color.Yellow("Problem %v: cannot build the submission URL. Skip.", problem.ID)
		case hasSubmission && ref.Ext == "":
			color.Yellow("Problem %v: language %q of submission #%v has no known file extension. Skip.",
				problem.ID, ref.Lang, ref.ID)
		case hasSubmission:
			target := filepath.Join(problem.Path, strings.ToLower(problem.ID)+"."+ref.Ext)
			if len(local) > 0 {
				note := fmt.Sprintf("Problem %v already has code (%v). Overwrite %v with submission #%v? (y/n) ",
					problem.ID, strings.Join(local, ", "), filepath.Base(target), ref.ID)
				if !util.YesOrNo(note) {
					color.Yellow("Skip %v", problem.ID)
					continue
				}
			}
			code, e := cln.FetchSubmissionCode(ref.URL)
			if e != nil {
				color.Red("Failed to fetch submission #%v: %v", ref.ID, e.Error())
				continue
			}
			if e = ioutil.WriteFile(target, []byte(code), 0644); e != nil {
				color.Red("Failed to save %v: %v", target, e.Error())
				continue
			}
			color.Green("Saved submission #%v -> %v", ref.ID, target)
		case len(local) == 0:
			if e := loadTemplate(); e != nil {
				return e
			}
			if e := gen(source, problem.Path, templateExt); e != nil {
				return e
			}
		default:
			color.Yellow("Problem %v: no submission, keep the local code (%v).",
				problem.ID, strings.Join(local, ", "))
		}
	}
	return nil
}
