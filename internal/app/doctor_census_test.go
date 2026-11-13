package app

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorCountsLogFilesNoSessionReads(t *testing.T) {
	w := newWorld(t)
	out, code := w.run("/", "doctor")
	if code != 0 || strings.Contains(out, "does not read") {
		t.Fatalf("every log in the world is read, so nothing is unread:\n%s", out)
	}
	if !strings.Contains(out, "files: all time") {
		t.Fatalf("the report must say what scope each number covers:\n%s", out)
	}

	journal := filepath.Join(w.claudeProject("/work/shop"), checkoutID, "subagents", "workflows", "wf_c3731a5c-f84", "journal.jsonl")
	w.write(journal, 0, "{}\n")
	out, _ = w.run("/", "doctor")
	if !strings.Contains(out, "1 log file this build does not read: */<id>/subagents/workflows/wf_<id>/journal.jsonl") || !strings.Contains(out, journal) {
		t.Fatalf("the journal must be reported as unread:\n%s", out)
	}

	out, _ = w.run("/", "doctor", "--json")
	var res struct {
		Reports []struct {
			Provider string `json:"provider"`
			Unread   []struct {
				Pattern string `json:"pattern"`
				Count   int    `json:"count"`
			} `json:"unread_files"`
		} `json:"health"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err, out)
	}
	found := 0
	for _, r := range res.Reports {
		for _, u := range r.Unread {
			if r.Provider != "claude" || u.Count != 1 {
				t.Fatalf("%s: %+v", r.Provider, u)
			}
			found++
		}
	}
	if found != 1 {
		t.Fatalf("want one unread kind in --json:\n%s", out)
	}

	for _, name := range []string{"a", "b", "c", "d", "e"} {
		w.write(filepath.Join(filepath.Dir(journal), "notes-"+name+".jsonl"), 0, "{}\n")
	}
	out, _ = w.run("/", "doctor")
	if !strings.Contains(out, "notes-a.jsonl") || strings.Contains(out, "notes-e.jsonl") || !strings.Contains(out, "+1 more kind\n") {
		t.Fatalf("six kinds of one file each: the first five by name, then a count:\n%s", out)
	}
}
