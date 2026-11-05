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
