package cmd

import (
	"errors"
	"fmt"
	"sort"

	"github.com/cys001x/cf-tool-x/client"
	"github.com/cys001x/cf-tool-x/config"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
)

// recentContests keeps finished regular contests, newest first. Gym contests
// are excluded because they have their own list.
func recentContests(all []client.ContestInfo, limit int) []client.ContestInfo {
	recent := make([]client.ContestInfo, 0, len(all))
	for _, contest := range all {
		if contest.Phase != "FINISHED" || contest.ID <= 0 || contest.ID >= 100000 {
			continue
		}
		recent = append(recent, contest)
	}
	sort.Slice(recent, func(i, j int) bool {
		return recent[i].StartTimeSeconds > recent[j].StartTimeSeconds
	})
	if limit > 0 && len(recent) > limit {
		recent = recent[:limit]
	}
	return recent
}

// NContests lists the most recent finished contests.
func NContests() error {
	cln := client.Instance
	if cln == nil {
		return errors.New("Codeforces client is not initialized")
	}
	all, err := cln.Contests()
	if err != nil {
		return err
	}
	recent := recentContests(all, 20)
	if len(recent) == 0 {
		return errors.New("No finished contests found")
	}
	host := "https://codeforces.com"
	if config.Instance != nil && config.Instance.Host != "" {
		host = config.Instance.Host
	}
	table := tablewriter.NewWriter(color.Output)
	table.SetHeader([]string{"link", "div", "name"})
	table.SetAutoWrapText(false)
	table.SetRowLine(false)
	for _, contest := range recent {
		div := contest.Div
		if div == "" {
			div = "-"
		}
		table.Append([]string{fmt.Sprintf("%v/contest/%v", host, contest.ID), div, contest.Name})
	}
	table.Render()
	return nil
}
