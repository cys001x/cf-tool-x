package client

import (
	"reflect"
	"testing"
)

func TestFindTags(t *testing.T) {
	body := []byte(`<html><body>
		<div class="roundbox">
			<div class="tags">
				<span class="tag-box" title="brute force">brute force</span>
				<span class="tag-box" title="dp">dp</span>
				<span class="tag-box" title="greedy">greedy</span>
				<span class="tag-box" style="font-size:1.2rem;" title="Difficulty">
    *1500
</span>
			</div>
		</div>
	</body></html>`)

	want := []string{"brute force", "dp", "greedy"}
	tags, difficulty := findTags(body)
	if !reflect.DeepEqual(tags, want) {
		t.Errorf("findTags tags = %v, want %v", tags, want)
	}
	if difficulty != "1500" {
		t.Errorf("findTags difficulty = %q, want %q", difficulty, "1500")
	}

	tags, difficulty = findTags([]byte(`<html>no tags here</html>`))
	if len(tags) != 0 || difficulty != "" {
		t.Errorf("findTags(no tags) = %v, %q, want empty", tags, difficulty)
	}
}

func TestRenderTags(t *testing.T) {
	want := "# Tags\n\n- dp\n- greedy\n\n**Difficulty:** 1500\n"
	if got := renderTags([]string{"dp", "greedy"}, "1500"); got != want {
		t.Errorf("renderTags = %q, want %q", got, want)
	}

	want = "# Tags\n\n- dp\n"
	if got := renderTags([]string{"dp"}, ""); got != want {
		t.Errorf("renderTags(no difficulty) = %q, want %q", got, want)
	}
}
