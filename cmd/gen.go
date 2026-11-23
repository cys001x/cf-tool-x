package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cys001x/cf-tool-x/client"
	"github.com/cys001x/cf-tool-x/config"
	"github.com/cys001x/cf-tool-x/util"
	"github.com/fatih/color"
)

func parseTemplate(source string, cln *client.Client) string {
	now := time.Now()
	source = strings.ReplaceAll(source, "$%U%$", cln.Handle)
	source = strings.ReplaceAll(source, "$%Y%$", fmt.Sprintf("%v", now.Year()))
	source = strings.ReplaceAll(source, "$%M%$", fmt.Sprintf("%02v", int(now.Month())))
	source = strings.ReplaceAll(source, "$%D%$", fmt.Sprintf("%02v", now.Day()))
	source = strings.ReplaceAll(source, "$%h%$", fmt.Sprintf("%02v", now.Hour()))
	source = strings.ReplaceAll(source, "$%m%$", fmt.Sprintf("%02v", now.Minute()))
	source = strings.ReplaceAll(source, "$%s%$", fmt.Sprintf("%02v", now.Second()))
	return source
}

func readTemplateSource(path string, cln *client.Client) (source string, err error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}
	source = parseTemplate(string(b), cln)
	return
}

// ensureAnalysisFile creates an empty "<prefix>_analysis.md" when it is
// missing. An existing file is never overwritten.
func ensureAnalysisFile(currentPath, prefix string) bool {
	name := client.PrefixedName(prefix, "analysis.md")
	path := filepath.Join(currentPath, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err == nil {
		file.Close()
		color.Green("Created %v.", name)
		return true
	}
	if !os.IsExist(err) {
		color.Yellow("Cannot create %v: %v", path, err)
	}
	return false
}

var practiceProblemReg = regexp.MustCompile(`^\d+[a-z][a-z0-9]*$`)

// problemPrefixFromPath derives the file-name prefix from a practice problem
// folder such as "{cf}/practice/drill/1136a" -> "1136a". Contest and gym
// folders are covered by the problem id parsed from the path, so this only
// needs to recognise the "<contest-id><index>" practice folder names.
func problemPrefixFromPath(dir string) string {
	base := strings.ToLower(filepath.Base(dir))
	if practiceProblemReg.MatchString(base) {
		return base
	}
	return ""
}

// gen writes the source file and an empty analysis file into currentPath. With
// a prefix the code file is named "<prefix><ext>" (e.g. "1136a.cpp"), otherwise
// after the directory.
func gen(source, currentPath, ext, prefix string) error {
	stem := filepath.Base(currentPath)
	if prefix != "" {
		stem = prefix
	}
	path := filepath.Join(currentPath, stem)

	savePath := path + ext
	i := 1
	for _, err := os.Stat(savePath); err == nil; _, err = os.Stat(savePath) {
		tmpPath := fmt.Sprintf("%v_%v%v", path, i, ext)
		fmt.Printf("%v exists. Rename to %v\n", filepath.Base(savePath), filepath.Base(tmpPath))
		savePath = tmpPath
		i++
	}

	err := ioutil.WriteFile(savePath, []byte(source), 0644)
	if err == nil {
		ensureAnalysisFile(currentPath, prefix)
		color.Green("Generated! See %v", filepath.Base(savePath))
	}
	return err
}

// Gen command
func Gen() (err error) {
	cfg := config.Instance
	if len(cfg.Template) == 0 {
		return errors.New("You have to add at least one code template by `cf config`")
	}
	alias := Args.Alias
	var path string

	if alias != "" {
		templates := cfg.TemplateByAlias(alias)
		if len(templates) < 1 {
			return fmt.Errorf("Cannot find any template with alias %v", alias)
		} else if len(templates) == 1 {
			path = templates[0].Path
		} else {
			fmt.Printf("There are multiple templates with alias %v\n", alias)
			for i, template := range templates {
				fmt.Printf(`%3v: "%v"`, i, template.Path)
				fmt.Println()
			}
			i := util.ChooseIndex(len(templates))
			path = templates[i].Path
		}
	} else {
		path = cfg.Template[cfg.Default].Path
	}

	cln := client.Instance
	source, err := readTemplateSource(path, cln)
	if err != nil {
		return
	}

	currentPath, err := os.Getwd()
	if err != nil {
		return
	}

	prefix := ""
	if Args.Info.ProblemID != "" {
		prefix = Args.Info.FilePrefix(Args.Info.ProblemID)
	} else {
		prefix = problemPrefixFromPath(currentPath)
	}

	ext := filepath.Ext(path)
	return gen(source, currentPath, ext, prefix)
}
