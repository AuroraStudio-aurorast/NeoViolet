package lyrics

import (
	"testing"
	"time"
)

// WordEnd resolves a word's span from the most specific source available: the
// fragment's own End, then the next fragment's Time, then the line's End.
func TestWordEnd_ResolutionOrder(t *testing.T) {
	sec := func(n int) time.Duration { return time.Duration(n) * time.Second }
	cases := []struct {
		name  string
		line  LyricLine
		index int
		want  time.Duration
	}{
		{
			name:  "own End wins",
			line:  LyricLine{Words: []WordFragment{{Time: sec(1), End: sec(2), Text: "a"}}},
			index: 0,
			want:  sec(2),
		},
		{
			name:  "falls back to the next fragment's Time",
			line:  LyricLine{Words: []WordFragment{{Time: sec(1), Text: "a"}, {Time: sec(3), Text: "b"}}},
			index: 0,
			want:  sec(3),
		},
		{
			name:  "last fragment falls back to the line's End",
			line:  LyricLine{End: sec(7), Words: []WordFragment{{Time: sec(1), Text: "a"}}},
			index: 0,
			want:  sec(7),
		},
		{
			name:  "unbounded line reports zero",
			line:  LyricLine{Words: []WordFragment{{Time: sec(1), Text: "a"}}},
			index: 0,
			want:  0,
		},
		{
			name:  "End <= Time is treated as unknown",
			line:  LyricLine{Words: []WordFragment{{Time: sec(5), End: sec(5), Text: "a"}, {Time: sec(9), Text: "b"}}},
			index: 0,
			want:  sec(9),
		},
		{
			name:  "index out of range reports zero",
			line:  LyricLine{Words: []WordFragment{{Time: sec(1), Text: "a"}}},
			index: 4,
			want:  0,
		},
		{
			name:  "negative index reports zero",
			line:  LyricLine{Words: []WordFragment{{Time: sec(1), Text: "a"}}},
			index: -1,
			want:  0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.line.WordEnd(tc.index); got != tc.want {
				t.Errorf("WordEnd(%d) = %v, want %v", tc.index, got, tc.want)
			}
		})
	}
}
