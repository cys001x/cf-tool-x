package client

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cys001x/cf-tool-x/util"
	"github.com/fatih/color"
)

// SyncedProblem describes one problem handled by Sync.
type SyncedProblem struct {
	ID         string
	Path       string
	Samples    int
	StandardIO bool
}

// LatestSubmission is the newest submission of one problem.
type LatestSubmission struct {
	ID   uint64
	Lang string
	Ext  string
	URL  string
}

// Sync downloads samples and statements for the problems described by info.
// Every problem page is fetched once and reused for both the samples and the
// statement. The newest submission of each problem is returned in a map keyed
// by the lower-case problem index; the map is empty when the account has no
// submissions or the problem type does not expose them (acmsguru).
func (c *Client) Sync(info Info) (problems []SyncedProblem, latest map[string]LatestSubmission, err error) {
	color.Cyan("Sync " + info.Hint())

	problemID := info.ProblemID
	info.ProblemID = "%v"
	urlFormatter, err := info.ProblemURL(c.host)
	if err != nil {
		return nil, nil, err
	}
	info.ProblemID = ""

	if problemID == "" {
		statics, statErr := c.Statis(info)
		if statErr != nil {
			return nil, nil, statErr
		}
		problems = make([]SyncedProblem, len(statics))
		for i, problem := range statics {
			problems[i].ID = problem.ID
		}
	} else {
		problems = []SyncedProblem{{ID: problemID}}
	}

	if latest, err = c.latestSubmissions(info); err != nil {
		return nil, nil, err
	}

	contestPath := info.Path()
	mu := sync.Mutex{}
	loginErr := error(nil)

	wg := sync.WaitGroup{}
	wg.Add(len(problems))
	for i := range problems {
		problems[i].Path = filepath.Join(contestPath, strings.ToLower(problems[i].ID))
		go func(problem *SyncedProblem) {
			defer wg.Done()
			mu.Lock()
			fmt.Printf("Syncing %v\n", problem.ID)
			mu.Unlock()

			if e := os.MkdirAll(problem.Path, os.ModePerm); e != nil {
				mu.Lock()
				color.Red("Failed %v: %v", problem.ID, e.Error())
				mu.Unlock()
				return
			}

			URL := fmt.Sprintf(urlFormatter, problem.ID)
			body, e := util.GetBody(c.client, URL)
			if e != nil {
				mu.Lock()
				color.Red("Failed %v: %v", problem.ID, e.Error())
				if e.Error() == ErrorNotLogged {
					loginErr = e
				}
				mu.Unlock()
				return
			}
			if _, e = findHandle(body); e != nil {
				mu.Lock()
				color.Red("Failed %v: %v", problem.ID, e.Error())
				loginErr = e
				mu.Unlock()
				return
			}

			samples, standardIO, e := writeSamples(body, problem.Path, &mu)
			if e != nil {
				mu.Lock()
				color.Red("Failed %v samples: %v", problem.ID, e.Error())
				mu.Unlock()
				return
			}

			statementErr := c.writeContentToMdFile(body, URL, problem.Path, "statement.md")
			mu.Lock()
			if statementErr != nil {
				color.Red("Failed %v statement: %v", problem.ID, statementErr.Error())
			} else {
				warn := ""
				if !standardIO {
					warn = " " + color.YellowString("Non standard input/output format.")
				}
				color.Green("Synced %v: %v samples, statement.md%v", problem.ID, samples, warn)
			}
			problem.Samples, problem.StandardIO = samples, standardIO
			mu.Unlock()
		}(&problems[i])
	}
	wg.Wait()

	if loginErr != nil {
		return nil, nil, loginErr
	}
	return problems, latest, nil
}

// latestSubmissions returns the newest submission of every problem of the
// contest described by info. The "my submissions" page lists newest first, so
// the first submission seen for a problem wins.
func (c *Client) latestSubmissions(info Info) (map[string]LatestSubmission, error) {
	latest := map[string]LatestSubmission{}

	URL, err := info.MySubmissionURL(c.host)
	if err != nil {
		// Problem types without a submission list (acmsguru) are not an error.
		return latest, nil
	}

	submissions, err := c.getSubmissions(URL, -1)
	if err != nil {
		if err.Error() == ErrorNotLogged {
			return nil, err
		}
		// No submissions yet is a normal state, not a failure.
		return latest, nil
	}

	for _, submission := range submissions {
		problemID := submission.ParseProblemIndex()
		if problemID == "" {
			continue
		}
		if _, ok := latest[problemID]; ok {
			continue
		}
		ref := LatestSubmission{ID: submission.id, Lang: submission.lang}
		ref.Ext, _ = LanguageExt(submission.lang)

		subInfo := info
		subInfo.ProblemID = problemID
		subInfo.SubmissionID = fmt.Sprintf("%v", submission.id)
		if submissionURL, e := subInfo.SubmissionURL(c.host); e == nil {
			ref.URL = submissionURL
		}
		latest[problemID] = ref
	}
	return latest, nil
}
