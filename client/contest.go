package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ContestProblemMeta is one problem of a contest returned by the Codeforces API.
type ContestProblemMeta struct {
	Index string `json:"index"`
	Name  string `json:"name"`
}

// ContestMeta is the contest metadata used by the local timer database.
type ContestMeta struct {
	ID               int64                `json:"id"`
	Name             string               `json:"name"`
	Div              string               `json:"-"`
	StartTimeSeconds int64                `json:"startTimeSeconds"`
	DurationSeconds  int64                `json:"durationSeconds"`
	Problems         []ContestProblemMeta `json:"problems"`
}

var contestDivReg = regexp.MustCompile(`(?i)\bDiv\.?\s*([123])(?:\s*\+\s*(?:Div\.?\s*)?([123]))?`)

func parseContestDiv(name string) string {
	m := contestDivReg.FindStringSubmatch(name)
	if m == nil {
		return ""
	}
	if m[2] != "" {
		return m[1] + "+" + m[2]
	}
	return m[1]
}

// ProblemsetProblem is one entry of the Codeforces problemset API.
type ProblemsetProblem struct {
	ContestID int64    `json:"contestId"`
	Index     string   `json:"index"`
	Name      string   `json:"name"`
	Rating    int      `json:"rating"`
	Tags      []string `json:"tags"`
}

// ProblemsetProblems fetches every problem listed on /problemset/problems.
func (c *Client) ProblemsetProblems() ([]ProblemsetProblem, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("Codeforces client is not initialized")
	}
	endpoint := strings.TrimRight(c.host, "/") + "/api/problemset.problems"
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	httpResp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Status  string `json:"status"`
		Comment string `json:"comment"`
		Result  struct {
			Problems []ProblemsetProblem `json:"problems"`
		} `json:"result"`
	}
	if err = json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if resp.Status != "OK" {
		if resp.Comment == "" {
			resp.Comment = "problemset.problems failed"
		}
		return nil, errors.New(resp.Comment)
	}
	return resp.Result.Problems, nil
}

// ContestInfo is one entry of the Codeforces contest.list API.
type ContestInfo struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Phase            string `json:"phase"`
	StartTimeSeconds int64  `json:"startTimeSeconds"`
	DurationSeconds  int64  `json:"durationSeconds"`
	Div              string `json:"-"`
}

// Contests fetches every contest known to the contest.list API.
func (c *Client) Contests() ([]ContestInfo, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("Codeforces client is not initialized")
	}
	endpoint := strings.TrimRight(c.host, "/") + "/api/contest.list"
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	httpResp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Status  string        `json:"status"`
		Comment string        `json:"comment"`
		Result  []ContestInfo `json:"result"`
	}
	if err = json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if resp.Status != "OK" {
		if resp.Comment == "" {
			resp.Comment = "contest.list failed"
		}
		return nil, errors.New(resp.Comment)
	}
	for i := range resp.Result {
		resp.Result[i].Div = parseContestDiv(resp.Result[i].Name)
	}
	return resp.Result, nil
}

// ContestMeta fetches a contest title, schedule, division and problem names
// from the public Codeforces API. Group contests and acmsguru are not covered
// by the API; callers should treat the error as "metadata unavailable".
func (c *Client) ContestMeta(contestID string) (*ContestMeta, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("Codeforces client is not initialized")
	}
	contestID = strings.TrimSpace(contestID)
	if contestID == "" {
		return nil, errors.New(ErrorNeedContestID)
	}
	endpoint := fmt.Sprintf("%v/api/contest.standings?contestId=%v",
		strings.TrimRight(c.host, "/"), url.QueryEscape(contestID))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	httpResp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Status  string `json:"status"`
		Comment string `json:"comment"`
		Result  struct {
			Contest  ContestMeta          `json:"contest"`
			Problems []ContestProblemMeta `json:"problems"`
		} `json:"result"`
	}
	if err = json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if resp.Status != "OK" {
		if resp.Comment == "" {
			resp.Comment = "contest.standings failed"
		}
		return nil, errors.New(resp.Comment)
	}
	meta := resp.Result.Contest
	meta.Problems = resp.Result.Problems
	meta.Div = parseContestDiv(meta.Name)
	return &meta, nil
}
