package catalog

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ryheff24/agentlog/internal/transcript"
)

func TestUnreadCountsTheLogFilesNoSessionLists(t *testing.T) {
	root := t.TempDir()
	put := func(rel string, body string) string {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	read := []string{
		put("-Users-me-shop/3f2a91bc-1111-4222-8333-444455556666.jsonl", "{}\n"),
		put("-Users-me-shop/3f2a91bc-1111-4222-8333-444455556666/subagents/agent-a1b2c3d4e5f60718.jsonl", "{}\n"),
		put("2026/11/02/rollout-2026-11-02T10-20-30-019a1cd3-e77d-78a3-9eee-b43182f1e133.jsonl.zst", "z"),
	}
	put("-Users-me-shop/3f2a91bc-1111-4222-8333-444455556666/subagents/workflows/wf_c3731a5c-f84/journal.jsonl", "{}\n")
	journal := put("-Users-me-blog/9e8d7c6b-2222-4333-8444-555566667777/subagents/workflows/wf_e1424d2b-598/journal.jsonl", "{}\n{}\n")
	notes := put("-Users-me-blog/9e8d7c6b-2222-4333-8444-555566667777/notes.jsonl", "{}\n")
	put("-Users-me-shop/3f2a91bc-1111-4222-8333-444455556666/notes.jsonl", "{}\n")
	put("-Users-me-shop/5a1e0000-0000-4000-8000-000000000001/notes.jsonl", "{}\n")
	put("-Users-me-shop/3f2a91bc-1111-4222-8333-444455556666/subagents/agent-a1b2c3d4e5f60718.meta.json", "{}")
	put("-Users-me-shop/3f2a91bc-1111-4222-8333-444455556666/notes.md", "#")

	p := &fakeProvider{name: transcript.Claude, roots: []string{root, filepath.Join(root, "missing")}}
	for _, f := range read {
		p.sessions = append(p.sessions, &transcript.Session{ID: filepath.Base(f), Files: []string{f}})
	}
	c := &Catalog{Providers: []Provider{p}}

	got, err := c.Unread(transcript.Claude)
	if err != nil {
		t.Fatal(err)
	}
	want := []FileKind{
		{Pattern: "*/<id>/notes.jsonl", Count: 3, Bytes: 9, Sample: notes},
		{Pattern: "*/<id>/subagents/workflows/wf_<id>/journal.jsonl", Count: 2, Bytes: 9, Sample: journal},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}

	p.err = os.ErrPermission
	if _, err := c.Unread(transcript.Claude); err == nil {
		t.Fatal("a discovery that failed must not report every file as unread")
	}
}

func TestKindOf(t *testing.T) {
	for in, want := range map[string]string{
		"-Users-me-shop/3f2a91bc-1111-4222-8333-444455556666.jsonl":                              "*/<id>.jsonl",
		"-Users-me-shop/3f2a91bc-1111-4222-8333-444455556666/subagents/agent-aland-u12ab.jsonl":  "*/<id>/subagents/agent-<id>.jsonl",
		"-Users-me-shop/3f2a91bc-1111-4222-8333-444455556666/subagents/agent-a1b2c3d4e5f6.jsonl": "*/<id>/subagents/agent-<id>.jsonl",
		"2026/11/02/rollout-2026-11-02T10-20-30-019a1cd3-e77d-78a3-9eee-b43182f1e133.jsonl.zst":  "*/<id>/<id>/rollout-<id>.jsonl.zst",
		"3f2a91bc-1111-4222-8333-444455556666/events.jsonl":                                      "*/events.jsonl",
		"history.jsonl": "history.jsonl",
	} {
		if got := kindOf(in); got != want {
			t.Errorf("kindOf(%q) = %q, want %q", in, got, want)
		}
	}
}
