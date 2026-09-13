package ui

import "testing"

// The anchor rule: the current line group starts one third down the box, and
// the group itself always has to fit.
func TestPanelFormat_AnchorTarget(t *testing.T) {
	f := panelFormat{AnchorNum: panelAnchorNum, AnchorDen: panelAnchorDen, MaxWrapRows: 2}
	cases := []struct {
		innerH, curRows, want int
	}{
		{13, 1, 4}, // 13/3, the 100x24 default box
		{13, 2, 4}, // a wrapped current line still starts at 4
		{14, 1, 4},
		{31, 1, 10}, // tall terminal
		{6, 1, 2},   // the smallest box the panel is shown in
		{3, 1, 1},
		{2, 1, 0}, // too short for a third: no room above
		{1, 1, 0},
		{13, 13, 0}, // the group itself fills the box
		{13, 20, 0}, // a group larger than the box cannot start above the top
		{0, 1, 0},
		{13, 0, 0},
	}
	for _, tc := range cases {
		if got := f.anchorTarget(tc.innerH, tc.curRows); got != tc.want {
			t.Errorf("anchorTarget(innerH=%d, curRows=%d) = %d, want %d",
				tc.innerH, tc.curRows, got, tc.want)
		}
	}
}

// Without a context cap the window fills the box: a side that runs out of lines
// hands its rows to the other one, but only when the box could be filled at all.
func TestPanelFormat_AnchorRows_FillMode(t *testing.T) {
	f := panelFormat{AnchorNum: panelAnchorNum, AnchorDen: panelAnchorDen, MaxWrapRows: 2}
	cases := []struct {
		name                          string
		innerH, curRows, above, below int
		want                          int
	}{
		{"mid song stays on the anchor", 13, 1, 20, 19, 4},
		{"start hugs the top", 13, 1, 0, 39, 0},
		{"end hugs the bottom", 13, 1, 39, 0, 12},
		{"one line above is enough to hug", 13, 1, 1, 38, 1},
		{"both sides exactly fill the box", 13, 1, 6, 6, 6},
		{"tall box", 31, 1, 40, 40, 10},
		{"short song keeps a stable anchor", 13, 1, 1, 1, 1},
		{"above equals the anchor and material is short", 13, 1, 4, 0, 4},
		{"two-line song keeps a stable anchor", 13, 2, 0, 0, 0},
		{"one row box", 1, 1, 5, 5, 0},
		{"zero height", 0, 1, 5, 5, 0},
		{"zero current rows", 13, 0, 5, 5, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := f.anchorRows(tc.innerH, tc.curRows, tc.above, tc.below); got != tc.want {
				t.Errorf("anchorRows(%d, %d, %d, %d) = %d, want %d",
					tc.innerH, tc.curRows, tc.above, tc.below, got, tc.want)
			}
		})
	}
}

// A per-side cap means a deliberately compact window: the group keeps its
// anchor and the rest of the box stays blank.
func TestPanelFormat_AnchorRows_Capped(t *testing.T) {
	f := panelFormat{AnchorNum: panelAnchorNum, AnchorDen: panelAnchorDen, ContextLines: 2, MaxWrapRows: 2}
	for _, tc := range []struct{ above, below, want int }{
		{2, 2, 4}, // two lines each side, anchored at a third
		{0, 40, 4},
		{40, 0, 4},
		{1, 1, 4},
	} {
		if got := f.anchorRows(13, 1, tc.above, tc.below); got != tc.want {
			t.Errorf("anchorRows(above=%d, below=%d) = %d, want %d", tc.above, tc.below, got, tc.want)
		}
	}
}

// fitAnchor is the shared clamp: it keeps a group inside the box, and both
// anchorTarget and anchorRows rely on it.
func TestPanelFormat_FitAnchor(t *testing.T) {
	for _, tc := range []struct{ target, innerH, curRows, want int }{
		{4, 13, 1, 4},  // already inside
		{9, 13, 6, 7},  // clamped to the last start that fits
		{-3, 13, 1, 0}, // never above the top
		{4, 13, 20, 0},
	} {
		if got := fitAnchor(tc.target, tc.innerH, tc.curRows); got != tc.want {
			t.Errorf("fitAnchor(%d, %d, %d) = %d, want %d",
				tc.target, tc.innerH, tc.curRows, got, tc.want)
		}
	}
}

// panelFormatFor is the only reader of the panel configuration.
func TestPanelFormatFor(t *testing.T) {
	for _, ctx := range []int{0, 2, 10} {
		m := panelModel(t, ctx)
		got := panelFormatFor(m)
		if got.ContextLines != ctx {
			t.Errorf("ContextLines = %d, want %d", got.ContextLines, ctx)
		}
		if got.AnchorNum != 1 || got.AnchorDen != 3 {
			t.Errorf("anchor = %d/%d, want the documented 1/3", got.AnchorNum, got.AnchorDen)
		}
		if got.MaxWrapRows != 2 {
			t.Errorf("MaxWrapRows = %d, want 2", got.MaxWrapRows)
		}
	}
}
