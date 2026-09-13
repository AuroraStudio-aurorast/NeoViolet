package lyrics

import (
	"testing"
	"time"
)

func visibleFixture() *Data {
	return &Data{
		Agents: map[string]string{"v1": "Alice", "v2": "Bob"},
		Lines: []LyricLine{
			{Time: 0, Text: "intro"},
			{Time: 1 * time.Second, Text: "   "},
			{Time: 2 * time.Second, Text: ""},
			{Time: 3 * time.Second, Text: "duet", Agent: "v2"},
			{Time: 4 * time.Second, Text: "outro"},
		},
	}
}

func TestVisibleLines_FiltersBlankAndKeepsIndices(t *testing.T) {
	d := visibleFixture()
	got := d.VisibleLines()

	wantTexts := []string{"intro", "duet", "outro"}
	wantIndex := []int{0, 3, 4}
	if len(got) != len(wantTexts) {
		t.Fatalf("got %d lines, want %d (%+v)", len(got), len(wantTexts), got)
	}
	for i := range wantTexts {
		if got[i].Line.Text != wantTexts[i] {
			t.Errorf("line %d text = %q, want %q", i, got[i].Line.Text, wantTexts[i])
		}
		if got[i].Index != wantIndex[i] {
			t.Errorf("line %d index = %d, want %d", i, got[i].Index, wantIndex[i])
		}
	}
}

func TestVisibleLines_AgentFilter(t *testing.T) {
	d := visibleFixture()
	d.AgentFilter = "v2"
	got := d.VisibleLines()
	if len(got) != 1 || got[0].Line.Text != "duet" || got[0].Index != 3 {
		t.Fatalf("agent filter result = %+v, want only duet at index 3", got)
	}
}

func TestVisibleLines_EmptyAndNil(t *testing.T) {
	if got := (&Data{}).VisibleLines(); got != nil {
		t.Errorf("empty data = %+v, want nil", got)
	}
	var nilData *Data
	if got := nilData.VisibleLines(); got != nil {
		t.Errorf("nil data = %+v, want nil", got)
	}
	allBlank := &Data{Lines: []LyricLine{{Text: " "}, {Text: "\t"}}}
	if got := allBlank.VisibleLines(); got != nil {
		t.Errorf("all-blank data = %+v, want nil", got)
	}
}

// The agent display prefix counts as content: a line whose text is only the
// agent prefix is still visible.
func TestVisibleLines_AgentPrefixCountsAsContent(t *testing.T) {
	d := &Data{
		Agents: map[string]string{"v1": "Alice"},
		Lines:  []LyricLine{{Time: 0, Text: "", Agent: "v1"}},
	}
	got := d.VisibleLines()
	if len(got) != 1 {
		t.Fatalf("got %d lines, want 1", len(got))
	}
	if got[0].Line.Text != "" {
		t.Errorf("text = %q, want empty", got[0].Line.Text)
	}
}
