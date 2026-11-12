package app

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	contentA = "ca110000-0000-4000-8000-000000000001"
	contentB = "cb110000-0000-4000-8000-000000000002"
	contentC = "cc110000-0000-4000-8000-000000000003"
)

func contentGrepWorld(t *testing.T) *world {
	t.Helper()
	w := newWorld(t)
	for _, id := range []string{contentA, contentB, contentC} {
		l := w.claude(id, "/work/notes", time.Hour)
		if id == contentA {
			l.user("-", "kestrel ledger")
		}
		if id == contentA {
			l.user("", "ledger only here")
		} else {
			l.user("", "ledger")
		}
		if id == contentC {
			l.add("", map[string]any{"type": "assistant", "message": map[string]any{"model": "claude-opus", "content": []any{map[string]any{"type": "tool_use", "id": "nest", "name": "Bash", "input": map[string]any{"command": "agentlog other#1 --full"}}}}})
			l.add("", map[string]any{"type": "user", "message": map[string]any{"content": []any{map[string]any{"type": "tool_result", "tool_use_id": "nest", "content": "#999 assistant kestrel"}}}})
		}
		l.save(filepath.Join(w.claudeProject("/work/notes"), id+".jsonl"), time.Hour)
	}
	return w
}

func TestContentGrep(t *testing.T) {
	w := contentGrepWorld(t)
	for _, tc := range []struct {
		name             string
		args, want, drop []string
		exact            string
		json             bool
	}{
		{name: "1_both", args: []string{"kestrel", "--content", "--grep", "ledger", "--count"}, want: []string{"2 matching events in 1 sessions"}},
		{name: "2_no_match", args: []string{"kestrel", "--content", "--grep", "zzqqxx_nomatch", "--count"}, want: []string{"0 matching events in 0 sessions"}},
		{name: "3_events", args: []string{"kestrel", "--content", "--grep", "ledger"}, want: []string{contentA[:8], "ledger only here"}, drop: []string{contentB[:8], contentC[:8]}},
		{name: "4_handbacks", args: []string{"kestrel", "--content", "--grep", "ledger", "--include-handbacks", "--count"}, want: []string{"3 matching events in 2 sessions"}},
		{name: "5_content_only", args: []string{"kestrel", "--content", "--count"}, exact: "1 matching events in 1 sessions\nreturned 32 bytes\n"},
		{name: "6_grep_only", args: []string{"--grep", "ledger", "--count"}, want: []string{"4 matching events in 3 sessions"}},
		{name: "7_pinned_A", args: []string{contentA, "kestrel", "--content", "--grep", "ledger", "--count"}, want: []string{"2 matching events in 1 sessions"}},
		{name: "7_pinned_B", args: []string{contentB, "kestrel", "--content", "--grep", "ledger", "--count"}, want: []string{"0 matching events in 0 sessions"}},
		{name: "8_json", args: []string{"kestrel", "--content", "--grep", "ledger", "--json"}, json: true},
		{name: "9_list_id", args: []string{contentB, "kestrel", "--content", "--grep", "ledger", "--list"}, drop: []string{contentB[:8]}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, code := w.run("/work/notes", tc.args...)
			if code != 0 {
				t.Fatalf("code %d: %s", code, out)
			}
			if tc.exact != "" && out != tc.exact {
				t.Errorf("got %q, want %q", out, tc.exact)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q: %s", want, out)
				}
			}
			for _, drop := range tc.drop {
				if strings.Contains(out, drop) {
					t.Errorf("includes %q: %s", drop, out)
				}
			}
			if tc.json {
				var response struct {
					Query   string   `json:"query"`
					Notices []string `json:"notices"`
				}
				if err := json.Unmarshal([]byte(out), &response); err != nil {
					t.Fatal(err, out)
				}
				if response.Query != "ledger" {
					t.Errorf("query = %q, want ledger", response.Query)
				}
				found := false
				for _, notice := range response.Notices {
					if strings.Contains(notice, "kestrel") && strings.Contains(notice, "--content") {
						found = true
					}
				}
				if !found {
					t.Errorf("missing words notice: %s", out)
				}
			}
		})
	}
}
