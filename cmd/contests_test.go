package cmd

import (
	"testing"

	"github.com/cys001x/cf-tool-x/client"
)

func TestRecentContests(t *testing.T) {
	all := []client.ContestInfo{
		{ID: 100, Phase: "FINISHED", StartTimeSeconds: 1000},
		{ID: 101, Phase: "FINISHED", StartTimeSeconds: 3000},
		{ID: 102, Phase: "BEFORE", StartTimeSeconds: 5000},
		{ID: 100001, Phase: "FINISHED", StartTimeSeconds: 4000},
		{ID: 103, Phase: "FINISHED", StartTimeSeconds: 2000},
	}
	recent := recentContests(all, 2)
	if len(recent) != 2 {
		t.Fatalf("recent = %+v", recent)
	}
	if recent[0].ID != 101 || recent[1].ID != 103 {
		t.Fatalf("order = %v, %v; want 101, 103", recent[0].ID, recent[1].ID)
	}

	allRecent := recentContests(all, 20)
	if len(allRecent) != 3 {
		t.Fatalf("finished regular contests = %v, want 3", len(allRecent))
	}
	for _, contest := range allRecent {
		if contest.ID >= 100000 || contest.Phase != "FINISHED" {
			t.Fatalf("unexpected contest: %+v", contest)
		}
	}
}

func TestUpcomingContests(t *testing.T) {
	all := []client.ContestInfo{
		{ID: 200, Phase: "BEFORE", StartTimeSeconds: 5000},
		{ID: 201, Phase: "CODING", StartTimeSeconds: 1000},
		{ID: 202, Phase: "BEFORE", StartTimeSeconds: 3000},
		{ID: 100002, Phase: "BEFORE", StartTimeSeconds: 2000},
		{ID: 203, Phase: "FINISHED", StartTimeSeconds: 4000},
	}
	upcoming := upcomingContests(all)
	if len(upcoming) != 2 {
		t.Fatalf("upcoming = %+v", upcoming)
	}
	if upcoming[0].ID != 202 || upcoming[1].ID != 200 {
		t.Fatalf("order = %v, %v; want 202, 200", upcoming[0].ID, upcoming[1].ID)
	}
	for _, contest := range upcoming {
		if contest.Phase != "BEFORE" || contest.ID >= 100000 {
			t.Fatalf("unexpected contest: %+v", contest)
		}
	}
}
