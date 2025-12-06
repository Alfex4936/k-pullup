package util

import (
	"sort"
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		text     string
		want     []*Match
	}{
		{
			name:     "Basic English",
			patterns: []string{"he", "she", "his", "hers"},
			text:     "ushers",
			want: []*Match{
				{Word: []byte("she"), Index: 1},
				{Word: []byte("he"), Index: 2},
				{Word: []byte("hers"), Index: 2},
			},
		},
		{
			name:     "Korean (Hangul)",
			patterns: []string{"감자", "고구마", "자", "구"},
			text:     "감자고구마",
			want: []*Match{
				{Word: []byte("감자"), Index: 0},
				{Word: []byte("자"), Index: 3}, // '자' starts at byte index 3 (UTF-8)
				{Word: []byte("고구마"), Index: 6},
				{Word: []byte("구"), Index: 9},
			},
		},
		{
			name:     "Japanese",
			patterns: []string{"こんにちは", "世界", "に"},
			text:     "こんにちは世界",
			want: []*Match{
				{Word: []byte("こんにちは"), Index: 0},
				{Word: []byte("に"), Index: 6}, // 'に' starts at byte index 6
				{Word: []byte("世界"), Index: 15},
			},
		},
		{
			name:     "No Matches",
			patterns: []string{"foo", "bar"},
			text:     "bazqux",
			want:     nil,
		},
		{
			name:     "Overlapping",
			patterns: []string{"ab", "bab", "bc"},
			text:     "abababa",
			want: []*Match{
				{Word: []byte("ab"), Index: 0},
				{Word: []byte("bab"), Index: 1},
				{Word: []byte("ab"), Index: 2},
				{Word: []byte("bab"), Index: 3},
				{Word: []byte("ab"), Index: 4},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := CompileStrings(tt.patterns)
			got := m.FindAllString(tt.text)

			// Sort results by Index, then by Word (lexicographically or by length)
			// to ensure deterministic comparison against 'want'.
			sort.Slice(got, func(i, j int) bool {
				if got[i].Index != got[j].Index {
					return got[i].Index < got[j].Index
				}
				return len(got[i].Word) < len(got[j].Word)
			})
			// Also sort 'want' to be safe (though it's defined sorted in test cases)
			sort.Slice(tt.want, func(i, j int) bool {
				if tt.want[i].Index != tt.want[j].Index {
					return tt.want[i].Index < tt.want[j].Index
				}
				return len(tt.want[i].Word) < len(tt.want[j].Word)
			})

			if len(got) != len(tt.want) {
				t.Errorf("FindAllString() got %d matches, want %d", len(got), len(tt.want))
				for i, match := range got {
					t.Logf("Got[%d]: %s at %d", i, string(match.Word), match.Index)
				}
				for i, match := range tt.want {
					t.Logf("Want[%d]: %s at %d", i, string(match.Word), match.Index)
				}
				return
			}

			for i := range got {
				if string(got[i].Word) != string(tt.want[i].Word) || got[i].Index != tt.want[i].Index {
					t.Errorf("Match %d mismatch: got { %s %d }, want { %s %d }",
						i, got[i].Word, got[i].Index, tt.want[i].Word, tt.want[i].Index)
				}
			}
		})
	}
}

func BenchmarkFindAll(b *testing.B) {
	patterns := []string{"foo", "bar", "baz", "qux", "quux", "corge", "grault", "garply", "waldo", "fred", "plugh", "xyzzy", "thud"}
	m := CompileStrings(patterns)
	text := strings.Repeat("foobar baz qux quux corge grault garply waldo fred plugh xyzzy thud ", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.FindAllString(text)
	}
}
