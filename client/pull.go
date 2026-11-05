package client

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cys001x/cf-tool-x/util"

	"github.com/fatih/color"
)

func findCode(body []byte) (string, error) {
	reg := regexp.MustCompile(`<pre[\s\S]*?>([\s\S]*?)</pre>`)
	tmp := reg.FindSubmatch(body)
	if tmp == nil {
		return "", errors.New("Cannot find any code")
	}
	return html.UnescapeString(string(tmp[1])), nil
}

func findMessage(body []byte) (string, error) {
	reg := regexp.MustCompile(`Codeforces.showMessage\("([^"]*)"\);\s*?Codeforces\.reformatTimes\(\);`)
	tmp := reg.FindSubmatch(body)
	if tmp != nil {
		return string(tmp[1]), nil
	}
	return "", errors.New("Cannot find any message")
}

// ErrorSkip error
const ErrorSkip = "Exists, skip"

// ErrorTooManyRequest error
const ErrorTooManyRequest = "Too many requests"

// FetchSubmissionCode downloads a submission page and returns the source code
// shown in it.
func (c *Client) FetchSubmissionCode(URL string) (code string, err error) {
	body, err := util.GetBody(c.client, URL)
	if err != nil {
		return
	}

	if bytes.Contains(body, []byte("Just a moment")) {
		return "", errors.New("Cloudflare returned a challenge page instead of codeforces.\n" +
			"Refresh the cookies and check the browser User-Agent with `cf config`")
	}

	if message, e := findMessage(body); e == nil {
		return "", errors.New(message)
	}

	return findCode(body)
}

// PullCode pull problem's code to path
func (c *Client) PullCode(URL, path, ext string, rename bool) (filename string, err error) {
	filename = path + ext
	if rename {
		i := 1
		for _, err := os.Stat(filename); err == nil; _, err = os.Stat(filename) {
			tmpPath := fmt.Sprintf("%v_%v%v", path, i, ext)
			filename = tmpPath
			i++
		}
	} else if _, err := os.Stat(filename); err == nil {
		return "", errors.New(ErrorSkip)
	}

	code, err := c.FetchSubmissionCode(URL)
	if err != nil {
		return
	}

	err = os.MkdirAll(filepath.Dir(filename), os.ModePerm)
	if err != nil {
		return
	}

	err = ioutil.WriteFile(filename, []byte(code), 0644)
	return
}

// Pull pull all latest codes or ac codes
func (c *Client) Pull(info Info, rootPath string, ac bool) (err error) {
	color.Cyan("Pull " + info.Hint())

	URL, err := info.MySubmissionURL(c.host)
	if err != nil {
		return
	}

	submissions, err := c.getSubmissions(URL, -1)
	if err != nil {
		return
	}

	used := []Submission{}

	for _, submission := range submissions {
		problemID := strings.ToLower(strings.Split(submission.name, " ")[0])
		if info.ProblemID != "" && strings.ToLower(info.ProblemID) != problemID {
			continue
		}
		if ac && !(strings.Contains(submission.status, "Accepted") || strings.Contains(submission.status, "Pretests passed")) {
			continue
		}
		ext, ok := LanguageExt(submission.lang)
		if !ok {
			continue
		}
		path := ""
		if info.ProblemID == "" {
			path = filepath.Join(rootPath, problemID, problemID)
		} else {
			path = filepath.Join(rootPath, problemID)
		}
		newInfo := info
		newInfo.SubmissionID = fmt.Sprintf("%v", submission.id)
		URL, err := newInfo.SubmissionURL(c.host)
		if err != nil {
			return err
		}
		filename, err := c.PullCode(
			URL,
			path,
			"."+ext,
			true,
		)
		if err == nil {
			color.Green(fmt.Sprintf(`Saved %v`, filename))
			used = append(used, submission)
		} else {
			color.Red(fmt.Sprintf(`Error %v: %v`, newInfo.Hint(), err.Error()))
		}
	}

	if len(used) == 0 {
		return errors.New("Cannot find any code to save")
	}

	color.Cyan("These submissions' codes have been saved.")
	maxline := 0
	display(used, "", true, &maxline, false)
	return nil
}
