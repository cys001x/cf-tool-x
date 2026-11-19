package cmd

import (
	"errors"
	"fmt"
	"sort"
	"time"

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

// upcomingContests keeps contests that have not started, soonest first. Gym
// contests are excluded, like the finished-contest list.
func upcomingContests(all []client.ContestInfo) []client.ContestInfo {
	upcoming := make([]client.ContestInfo, 0, len(all))
	for _, contest := range all {
		if contest.Phase != "BEFORE" || contest.ID <= 0 || contest.ID >= 100000 {
			continue
		}
		upcoming = append(upcoming, contest)
	}
	sort.Slice(upcoming, func(i, j int) bool {
		return upcoming[i].StartTimeSeconds < upcoming[j].StartTimeSeconds
	})
	return upcoming
}

// FutureContests lists the contests that have not started yet.
func FutureContests() error {
	cln := client.Instance
	if cln == nil {
		return errors.New("Codeforces client is not initialized")
	}
	all, err := cln.Contests()
	if err != nil {
		return err
	}
	upcoming := upcomingContests(all)
	if len(upcoming) == 0 {
		return errors.New("No upcoming contests found")
	}
	host := "https://codeforces.com"
	if config.Instance != nil && config.Instance.Host != "" {
		host = config.Instance.Host
	}
	table := tablewriter.NewWriter(color.Output)
	table.SetHeader([]string{"link", "start", "div", "name"})
	table.SetAutoWrapText(false)
	table.SetRowLine(false)
	for _, contest := range upcoming {
		div := contest.Div
		if div == "" {
			div = "-"
		}
		start := time.Unix(contest.StartTimeSeconds, 0).Local().Format("2006-01-02 15:04")
		table.Append([]string{fmt.Sprintf("%v/contest/%v", host, contest.ID), start, div, contest.Name})
	}
	table.Render()
	return nil
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
