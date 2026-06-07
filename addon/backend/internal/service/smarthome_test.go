package service

import "testing"

// ── joinReplies ───────────────────────────────────────────────────────────────

func TestJoinReplies(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want string
	}{
		{
			name: "normal case — no trailing period",
			a:    "Свет включён",
			b:    "Музыка запущена",
			want: "Свет включён. Музыка запущена",
		},
		{
			name: "a ends with period — no double period",
			a:    "Свет включён.",
			b:    "Музыка запущена",
			want: "Свет включён. Музыка запущена",
		},
		{
			name: "a ends with period and space — no double period",
			a:    "Свет включён. ",
			b:    "Музыка запущена",
			want: "Свет включён. Музыка запущена",
		},
		{
			name: "a ends with multiple spaces and periods — still single period",
			a:    "Свет включён... ",
			b:    "Музыка запущена",
			want: "Свет включён. Музыка запущена",
		},
		{
			name: "b is empty string",
			a:    "Свет включён",
			b:    "",
			want: "Свет включён. ",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := joinReplies(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("joinReplies(%q, %q) = %q, want %q", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// ── withOpenQuestion ──────────────────────────────────────────────────────────

func TestWithOpenQuestion(t *testing.T) {
	tests := []struct {
		name       string
		reply      string
		hasQuestion bool // should append a question
	}{
		{
			name:        "normal reply without question — appends a question",
			reply:       "Свет выключен",
			hasQuestion: true,
		},
		{
			name:        "reply already ends with question mark — no change",
			reply:       "Какую музыку включить?",
			hasQuestion: false,
		},
		{
			name:        "reply with trailing spaces — spaces trimmed then question appended",
			reply:       "Свет выключен   ",
			hasQuestion: true,
		},
		{
			name:        "reply already ends with question mark and space — no change",
			reply:       "Какую музыку включить? ",
			hasQuestion: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := withOpenQuestion(tc.reply)
			if got == "" {
				t.Fatal("withOpenQuestion returned empty string")
			}
			if tc.hasQuestion {
				if !containsAny(got, openQuestions[:]) {
					t.Errorf("withOpenQuestion(%q) = %q, expected one of known questions appended", tc.reply, got)
				}
			} else {
				if got != tc.reply {
					t.Errorf("withOpenQuestion(%q) = %q, expected no change", tc.reply, got)
				}
			}
		})
	}
}

func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if len(s) > len(sub) && s[len(s)-len(sub):] == sub {
			return true
		}
	}
	return false
}
