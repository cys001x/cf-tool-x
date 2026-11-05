package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/cys001x/cf-tool-x/client"
	"github.com/cys001x/cf-tool-x/config"
	"github.com/fatih/color"
)

// practiceProblem is one selected problem and the data needed to materialize it.
type practiceProblem struct {
	Key     string      `json:"key"`
	Display string      `json:"display"`
	Name    string      `json:"name,omitempty"`
	Link    string      `json:"link,omitempty"`
	Status  string      `json:"status,omitempty"`
	Info    client.Info `json:"-"`
}

type practiceManifest struct {
	Name      string            `json:"name"`
	CreatedAt string            `json:"created_at"`
	Problems  []practiceProblem `json:"problems"`
}

func practiceSetDir(set string) string {
	return filepath.Join(practiceBaseDir(), "practice", sanitizePracticeName(set))
}

// resolvePracticeTimer maps "p_<set> <specifier>" to the problem folder inside
// the practice set. It rejects a problem that is not part of the set.
func resolvePracticeTimer(practice, specifier string) (info client.Info, dir, problem string, err error) {
	if strings.TrimSpace(specifier) == "" {
		return info, "", "", errors.New("Missing specifier, e.g. `cf timer start p_drill 1136a <note>`")
	}
	set := strings.TrimPrefix(strings.TrimSpace(practice), "p_")
	if set == "" {
		return info, "", "", errors.New("Missing practice set name, e.g. `cf timer start p_drill 1136a`")
	}
	base := practiceSetDir(set)
	if st, e := os.Stat(base); e != nil || !st.IsDir() {
		return info, "", "", fmt.Errorf("Practice set %q not found at %v", set, base)
	}
	info, err = resolveTimerInfo(specifier)
	if err != nil {
		return info, "", "", err
	}
	key := keysFromInfo(info).ProblemKey
	display := ""
	if data, e := os.ReadFile(filepath.Join(base, "practice.json")); e == nil {
		var manifest practiceManifest
		if json.Unmarshal(data, &manifest) == nil {
			for _, problem := range manifest.Problems {
				if problem.Key == key {
					display = problem.Display
					break
				}
			}
		}
	}
	if display == "" {
		display = strings.ToLower(info.ContestID + info.ProblemID)
	}
	if display == "" {
		display = strings.ToLower(info.ProblemID)
	}
	dir = filepath.Join(base, sanitizePracticeName(display))
	if st, e := os.Stat(dir); e != nil || !st.IsDir() {
		return info, "", "", fmt.Errorf("Problem %v is not in practice set %q (%v)", specifier, set, dir)
	}
	return info, dir, strings.ToLower(display), nil
}

// Practice command
func Practice() (err error) {
	if Args.Count <= 0 {
		return errors.New("The problem count must be greater than zero")
	}
	var problems []practiceProblem
	switch {
	case Args.LPractice:
		problems, err = selectLocalPractice("", Args.Count)
	case Args.LRePractice:
		problems, err = selectLocalPractice(problemStatusAccepted, Args.Count)
	case Args.LNRePractice:
		problems, err = selectLocalPractice(problemStatusPending, Args.Count)
	case Args.OPractice:
		problems, err = selectOnlinePractice(Args.Count, Args.Difficulty, Args.Tags)
	default:
		return errors.New("Unknown practice command")
	}
	if err != nil {
		return err
	}
	if len(problems) == 0 {
		return errors.New("No matching problems")
	}
	if len(problems) < Args.Count {
		color.Yellow("Only %v matching problem(s) available, wanted %v.", len(problems), Args.Count)
	}
	return createPractice(problems, Args.PracticeName)
}

func practiceHost() string {
	if config.Instance != nil && config.Instance.Host != "" {
		return config.Instance.Host
	}
	return "https://codeforces.com"
}

// practiceBaseDir returns the folder that contains contest/, gym/ and practice/.
func practiceBaseDir() string {
	if root := findCurrentRoot(); root != "" {
		return root
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	rootName := "cf"
	if config.Instance != nil && config.Instance.FolderName["root"] != "" {
		rootName = config.Instance.FolderName["root"]
	}
	return filepath.Join(cwd, rootName)
}

func parsePracticeKey(key string) (client.Info, bool) {
	parts := strings.Split(key, ":")
	if len(parts) < 3 {
		return client.Info{}, false
	}
	info := client.Info{ProblemID: strings.ToLower(parts[len(parts)-1])}
	switch parts[0] {
	case "contest":
		if len(parts) != 3 {
			return client.Info{}, false
		}
		info.ProblemType = "contest"
		info.ContestID = parts[1]
	case "gym":
		if len(parts) != 3 {
			return client.Info{}, false
		}
		info.ProblemType = "gym"
		info.ContestID = parts[1]
	case "group":
		if len(parts) != 4 {
			return client.Info{}, false
		}
		info.ProblemType = "group"
		info.GroupID = parts[1]
		info.ContestID = parts[2]
	case "acmsguru":
		if len(parts) != 3 {
			return client.Info{}, false
		}
		info.ProblemType = "acmsguru"
		info.ContestID = parts[1]
	default:
		return client.Info{}, false
	}
	return info, true
}

func problemLinkFor(info client.Info, link string) string {
	if link != "" {
		return link
	}
	if URL, err := info.ProblemURL(practiceHost()); err == nil {
		return URL
	}
	return ""
}

// selectLocalPractice picks random problems from the timer database. An empty
// status means every problem in the database.
func selectLocalPractice(status string, count int) ([]practiceProblem, error) {
	db, err := openTimerDB(true)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	query := `SELECT key, display, name, link, status FROM problem`
	var params []interface{}
	if status == problemStatusAccepted {
		// lrepractice also includes problems accepted again in practice mode.
		query += ` WHERE (status = ? OR status LIKE ?)`
		params = append(params, status, problemStatusPractice+"%")
	} else if status != "" {
		query += ` WHERE status = ?`
		params = append(params, status)
	}
	query += ` ORDER BY RANDOM() LIMIT ?`
	params = append(params, count)
	rows, err := db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []practiceProblem
	for rows.Next() {
		var problem practiceProblem
		if err = rows.Scan(&problem.Key, &problem.Display, &problem.Name, &problem.Link, &problem.Status); err != nil {
			return nil, err
		}
		info, ok := parsePracticeKey(problem.Key)
		if !ok {
			continue
		}
		problem.Info = info
		if problem.Display == "" {
			problem.Display = strings.ToLower(info.ContestID + info.ProblemID)
			if problem.Display == "" {
				problem.Display = info.ProblemID
			}
		}
		problem.Link = problemLinkFor(info, problem.Link)
		result = append(result, problem)
	}
	return result, rows.Err()
}

// parseDifficultyRange parses "", "1200" or "800-1200".
func parseDifficultyRange(raw string) (minRating, maxRating int, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, nil
	}
	raw = strings.ReplaceAll(raw, "..", "-")
	parts := strings.Split(raw, "-")
	if len(parts) > 2 {
		return 0, 0, fmt.Errorf("Invalid difficulty %q, use 1200 or 800-1200", raw)
	}
	minRating, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("Invalid difficulty %q, use 1200 or 800-1200", raw)
	}
	maxRating = minRating
	if len(parts) == 2 {
		maxRating, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return 0, 0, fmt.Errorf("Invalid difficulty %q, use 1200 or 800-1200", raw)
		}
	}
	if minRating < 0 || maxRating < 0 || minRating > maxRating {
		return 0, 0, fmt.Errorf("Invalid difficulty range %q", raw)
	}
	return minRating, maxRating, nil
}

func parsePracticeTags(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	seen := map[string]bool{}
	var tags []string
	for _, tag := range fields {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag != "" && !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	return tags
}

// filterOnlineProblems keeps problems in the rating range and with at least
// one of the requested tags.
func filterOnlineProblems(all []client.ProblemsetProblem, minRating, maxRating int, tags []string) []client.ProblemsetProblem {
	var result []client.ProblemsetProblem
	for _, problem := range all {
		if problem.ContestID == 0 || problem.Index == "" {
			continue
		}
		if minRating > 0 && (problem.Rating < minRating || problem.Rating > maxRating) {
			continue
		}
		if len(tags) > 0 {
			matched := false
			for _, tag := range problem.Tags {
				for _, wanted := range tags {
					if strings.EqualFold(tag, wanted) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				continue
			}
		}
		result = append(result, problem)
	}
	return result
}

func selectOnlinePractice(count int, difficulty, tagsRaw string) ([]practiceProblem, error) {
	cln := client.Instance
	if cln == nil {
		return nil, errors.New("Codeforces client is not initialized")
	}
	minRating, maxRating, err := parseDifficultyRange(difficulty)
	if err != nil {
		return nil, err
	}
	all, err := cln.ProblemsetProblems()
	if err != nil {
		return nil, err
	}
	filtered := filterOnlineProblems(all, minRating, maxRating, parsePracticeTags(tagsRaw))
	if len(filtered) == 0 {
		return nil, errors.New("No Codeforces problems match the filters")
	}
	rand.Shuffle(len(filtered), func(i, j int) {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	})
	if count > len(filtered) {
		count = len(filtered)
	}
	result := make([]practiceProblem, 0, count)
	for _, problem := range filtered[:count] {
		info := client.Info{
			ProblemType: normalizeProblemType("", strconv.FormatInt(problem.ContestID, 10)),
			ContestID:   strconv.FormatInt(problem.ContestID, 10),
			ProblemID:   strings.ToLower(problem.Index),
		}
		display := strings.ToLower(info.ContestID + problem.Index)
		result = append(result, practiceProblem{
			Key:     formatContestKey(info.ProblemType, info.ContestID, "") + ":" + info.ProblemID,
			Display: display,
			Name:    problem.Name,
			Status:  problemStatusPending,
			Link:    problemLinkFor(info, ""),
			Info:    info,
		})
	}
	return result, nil
}

func sanitizePracticeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, name)
	name = strings.Trim(name, "._")
	if name == "" {
		return "practice"
	}
	return name
}

// createPractice downloads samples and statements and generates code for the
// selected problems under <root>/practice/<name>.
func createPractice(problems []practiceProblem, name string) error {
	if name == "" {
		name = time.Now().Format("20060102-150405") + "-practice"
	}
	name = sanitizePracticeName(name)
	base := filepath.Join(practiceBaseDir(), "practice", name)
	if err := os.MkdirAll(base, 0755); err != nil {
		return err
	}
	color.Cyan("Practice folder: %v", base)

	cln := client.Instance
	source, ext := "", ""
	if config.Instance != nil && len(config.Instance.Template) > 0 && cln != nil {
		path := config.Instance.Template[config.Instance.Default].Path
		if s, err := readTemplateSource(path, cln); err == nil {
			source = s
			ext = filepath.Ext(path)
		}
	}

	success := 0
	for _, problem := range problems {
		dir := filepath.Join(base, sanitizePracticeName(problem.Display))
		if err := os.MkdirAll(dir, 0755); err != nil {
			color.Red("Failed %v: %v", problem.Display, err)
			continue
		}
		URL := problem.Link
		if URL == "" {
			URL = problemLinkFor(problem.Info, "")
		}
		if URL == "" || cln == nil {
			color.Red("Failed %v: no problem URL", problem.Display)
			continue
		}
		samples, standardIO, err := cln.ParseProblem(URL, dir, nil)
		if err != nil {
			color.Red("Failed %v: %v", problem.Display, err)
			continue
		}
		warn := ""
		if !standardIO {
			warn = color.YellowString(" Non standard input/output format.")
		}
		if err = cln.FetchUrlContentToMdFile(URL, dir, "statement.md", nil); err != nil {
			color.Yellow("Statement for %v skipped: %v", problem.Display, err)
		}
		if source != "" {
			gen(source, dir, ext)
		}
		color.Green("Practice %v: %v samples.%v", problem.Display, samples, warn)
		success++
	}

	manifest := practiceManifest{
		Name:      name,
		CreatedAt: time.Now().Format(timeLayout),
		Problems:  problems,
	}
	if data, err := json.MarshalIndent(manifest, "", "  "); err == nil {
		if err = writeFileAtomic(filepath.Join(base, "practice.json"), append(data, '\n')); err != nil {
			color.Yellow("Cannot write practice.json: %v", err)
		}
	}
	if success == 0 {
		return errors.New("No problem could be downloaded")
	}
	color.Green("Practice ready: %v (%v/%v problems).", base, success, len(problems))
	return nil
}
