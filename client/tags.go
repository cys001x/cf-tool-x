package client

import (
	"fmt"
	"html"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/cys001x/cf-tool-x/util"
	"github.com/fatih/color"
)

// TagsFileName is the file that stores the tags of one problem.
const TagsFileName = "tag.md"

var tagBoxReg = regexp.MustCompile(`class="tag-box[^"]*"([^>]*)>([\s\S]*?)</`)
var anyTagReg = regexp.MustCompile(`<[^>]*>`)

// findTags returns the tag names and the difficulty rating shown in the
// "Problem tags" block of a problem page. The difficulty is rendered there as
// a tag-box whose title is "Difficulty" and whose text is "*1500".
func findTags(body []byte) (tags []string, difficulty string) {
	seen := map[string]bool{}
	for _, m := range tagBoxReg.FindAllSubmatch(body, -1) {
		attrs := string(m[1])
		tag := anyTagReg.ReplaceAllString(string(m[2]), "")
		tag = strings.TrimSpace(html.UnescapeString(tag))
		if tag == "" {
			continue
		}
		if strings.Contains(attrs, `title="Difficulty"`) {
			difficulty = strings.TrimPrefix(tag, "*")
			continue
		}
		if seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	return
}

// renderTags formats tags and the difficulty rating as markdown.
func renderTags(tags []string, difficulty string) string {
	var sb strings.Builder
	sb.WriteString("# Tags\n\n")
	for _, tag := range tags {
		sb.WriteString("- ")
		sb.WriteString(tag)
		sb.WriteString("\n")
	}
	if difficulty != "" {
		sb.WriteString("\n**Difficulty:** ")
		sb.WriteString(difficulty)
		sb.WriteString("\n")
	}
	return sb.String()
}

// Tags fetches the tags and difficulty of the problems described by info and
// writes them to "tag.md" in each problem folder.
func (c *Client) Tags(info Info) (err error) {
	color.Cyan("Fetch tags " + info.Hint())

	problemID := info.ProblemID
	info.ProblemID = "%v"
	urlFormatter, err := info.ProblemURL(c.host)
	if err != nil {
		return
	}
	info.ProblemID = ""

	var problems []string
	if problemID == "" {
		statics, statErr := c.Statis(info)
		if statErr != nil {
			return statErr
		}
		for _, p := range statics {
			problems = append(problems, p.ID)
		}
	} else {
		problems = []string{problemID}
	}

	contestPath := info.Path()
	mu := sync.Mutex{}
	loginErr := error(nil)

	wg := sync.WaitGroup{}
	wg.Add(len(problems))
	for _, pid := range problems {
		go func(pid string) {
			defer wg.Done()
			ppath := problemDir(info.Dir, contestPath, pid)
			if e := os.MkdirAll(ppath, os.ModePerm); e != nil {
				mu.Lock()
				color.Red("Failed %v: %v", pid, e.Error())
				mu.Unlock()
				return
			}
			URL := fmt.Sprintf(urlFormatter, pid)
			body, e := util.GetBody(c.client, URL)
			if e != nil {
				mu.Lock()
				color.Red("Failed %v: %v", pid, e.Error())
				if e.Error() == ErrorNotLogged {
					loginErr = e
				}
				mu.Unlock()
				return
			}
			if _, e = findHandle(body); e != nil {
				mu.Lock()
				color.Red("Failed %v: %v", pid, e.Error())
				loginErr = e
				mu.Unlock()
				return
			}

			tags, difficulty := findTags(body)
			target := filepath.Join(ppath, TagsFileName)
			mu.Lock()
			defer mu.Unlock()
			if len(tags) == 0 && difficulty == "" {
				color.Yellow("Problem %v: no tags found. Tags may be hidden while the contest is running.", pid)
				return
			}
			if e = ioutil.WriteFile(target, []byte(renderTags(tags, difficulty)), 0644); e != nil {
				color.Red("Failed to save %v: %v", target, e.Error())
				return
			}
			color.Green("Saved %v tags -> %v", pid, target)
		}(pid)
	}
	wg.Wait()

	if loginErr != nil {
		return loginErr
	}
	return nil
}
