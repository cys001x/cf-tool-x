package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	"github.com/docopt/docopt-go"

	"github.com/cys001x/cf-tool-x/client"
	"github.com/cys001x/cf-tool-x/config"
	"github.com/cys001x/cf-tool-x/util"
	"github.com/fatih/color"
)

// Eval opts
func Eval(opts docopt.Opts) error {
	Args = &ParsedArgs{}
	opts.Bind(Args)
	if err := parseArgs(opts); err != nil {
		return err
	}
	if Args.Config {
		return Config()
	} else if Args.Submit {
		return Submit()
	} else if Args.List {
		return List()
	} else if Args.NContests {
		return NContests()
	} else if Args.Future {
		return FutureContests()
	} else if Args.Parse {
		return Parse()
	} else if Args.Gen {
		return Gen()
	} else if Args.Test {
		return Test()
	} else if Args.Watch {
		return Watch()
	} else if Args.Open {
		return Open()
	} else if Args.Stand {
		return Stand()
	} else if Args.Sid {
		return Sid()
	} else if Args.Detail {
		return Detail()
	} else if Args.Race {
		return Race()
	} else if Args.Pull {
		return Pull()
	} else if Args.Sync {
		return Sync()
	} else if Args.Clone {
		return Clone()
	} else if Args.Statement {
		return Statement()
	} else if Args.Material {
		return Material()
	} else if Args.Upgrade {
		return Upgrade()
	} else if Args.Completion {
		return Completion()
	} else if Args.Start {
		return StartTimer()
	} else if Args.End {
		return EndTimer()
	} else if Args.DB {
		return DB()
	} else if Args.LPractice || Args.LRePractice || Args.LNRePractice || Args.OPractice {
		return Practice()
	}
	return nil
}

// sampleFile identifies one sample pair in a test directory. Sample files are
// named "<prefix>_inK.txt" / "<prefix>_ansK.txt"; prefix is empty for the
// legacy "inK.txt" / "ansK.txt" names.
type sampleFile struct {
	prefix string
	index  string
}

func sampleName(prefix, kind, index string) string {
	if prefix == "" {
		return kind + index + ".txt"
	}
	return prefix + "_" + kind + index + ".txt"
}

func (s sampleFile) inName() string  { return sampleName(s.prefix, "in", s.index) }
func (s sampleFile) ansName() string { return sampleName(s.prefix, "ans", s.index) }

// getSampleID lists the sample pairs in dir. When preferredPrefix matches some
// files, only those are returned so that one directory can hold several
// problems' samples.
func getSampleID(dir, preferredPrefix string) (samples []sampleFile) {
	path := dir
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return
		}
	}
	paths, err := ioutil.ReadDir(path)
	if err != nil {
		return
	}
	prefixed := regexp.MustCompile(`^([A-Za-z0-9]+)_in(\d+)\.txt$`)
	legacy := regexp.MustCompile(`^in(\d+)\.txt$`)
	var found []sampleFile
	for _, f := range paths {
		name := f.Name()
		if m := prefixed.FindStringSubmatch(name); m != nil {
			found = append(found, sampleFile{prefix: m[1], index: m[2]})
		} else if m := legacy.FindStringSubmatch(name); m != nil {
			found = append(found, sampleFile{index: m[1]})
		}
	}
	if preferredPrefix != "" {
		var matched []sampleFile
		for _, s := range found {
			if s.prefix == preferredPrefix {
				matched = append(matched, s)
			}
		}
		if len(matched) > 0 {
			found = matched
		}
	}
	for _, s := range found {
		if _, err := os.Stat(filepath.Join(path, s.ansName())); err == nil {
			samples = append(samples, s)
		}
	}
	return
}

// CodeList Name matches some template suffix, index are template array indexes
type CodeList struct {
	Name  string
	Index []int
}

func getCode(filename, dir string, templates []config.CodeTemplate) (codes []CodeList, err error) {
	mp := make(map[string][]int)
	for i, temp := range templates {
		suffixMap := map[string]bool{}
		for _, suffix := range temp.Suffix {
			if _, ok := suffixMap[suffix]; !ok {
				suffixMap[suffix] = true
				sf := "." + suffix
				mp[sf] = append(mp[sf], i)
			}
		}
	}

	if filename != "" {
		ext := filepath.Ext(filename)
		if idx, ok := mp[ext]; ok {
			return []CodeList{CodeList{filename, idx}}, nil
		}
		return nil, fmt.Errorf("%v can not match any template. You could add a new template by `cf config`", filename)
	}

	scanDirs := [3]string{dir, Args.Info.Path(), ""}
	wd, err := os.Getwd()
	if err == nil {
		scanDirs[2] = wd
	}
	for _, scanDir := range scanDirs {
		if scanDir == "" {
			continue
		}
		paths, err := ioutil.ReadDir(scanDir)
		if err != nil {
			continue
		}

		for _, f := range paths {
			name := f.Name()
			ext := filepath.Ext(name)
			if idx, ok := mp[ext]; ok {
				codes = append(codes, CodeList{filepath.Join(scanDir, name), idx})
			}
		}

		return codes, nil
	}
	return nil, nil
}

func getOneCode(filename, dir string, templates []config.CodeTemplate) (name string, index int, err error) {
	codes, err := getCode(filename, dir, templates)
	if err != nil {
		return
	}
	if len(codes) < 1 {
		return "", 0, errors.New("Cannot find any code.\nMaybe you should add a new template by `cf config`")
	}
	if len(codes) > 1 {
		color.Cyan("There are multiple files can be selected.")
		for i, code := range codes {
			fmt.Printf("%3v: %v\n", i, code.Name)
		}
		i := util.ChooseIndex(len(codes))
		codes[0] = codes[i]
	}
	if len(codes[0].Index) > 1 {
		color.Cyan("There are multiple languages match the file.")
		for i, idx := range codes[0].Index {
			fmt.Printf("%3v: %v\n", i, client.Langs[templates[idx].Lang])
		}
		i := util.ChooseIndex(len(codes[0].Index))
		codes[0].Index[0] = codes[0].Index[i]
	}
	return codes[0].Name, codes[0].Index[0], nil
}

func loginAgain(cln *client.Client, err error) error {
	if err != nil && err.Error() == client.ErrorNotLogged {
		color.Red("Not logged. Try to login\n")
		err = cln.Login()
	}
	return err
}
