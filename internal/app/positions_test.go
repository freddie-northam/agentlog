package app

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ryheff24/agentlog/internal/catalog"
	"github.com/Ryheff24/agentlog/internal/claude"
	"github.com/Ryheff24/agentlog/internal/codex"
	"github.com/Ryheff24/agentlog/internal/copilot"
	"github.com/Ryheff24/agentlog/internal/jsonl"
	"github.com/Ryheff24/agentlog/internal/transcript"
	"github.com/Ryheff24/agentlog/internal/zstd/zstdtest"
)

// Every event must lead back to the line of the record it came from, so an agent can read that record itself.
// The checks recount each position from the raw bytes of the log, never from agentlog.
// The world gains a compressed Codex rollout and a Claude session with a log in two folders, and every session is read whole and through head and tail windows.
func TestPositions(t *testing.T) {
	w := newWorld(t)
	w.positionCases()
	logs := map[string]string{}
	content := func(path string) string {
		if s, ok := logs[path]; ok {
			return s
		}
		f, err := jsonl.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			t.Fatal(err)
		}
		logs[path] = string(b)
		return logs[path]
	}
	seen := map[string]int{}
	for _, p := range []catalog.Provider{claude.New(), codex.New(), copilot.New()} {
		ss, err := p.Discover(transcript.Query{Now: w.now})
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range ss {
			st, err := os.Stat(s.File())
			if err != nil {
				t.Fatal(err)
			}
			for _, win := range []jsonl.Window{{}, {HeadBytes: st.Size() / 2}, {TailBytes: st.Size() / 2}, {TailBytes: st.Size() * 2}} {
				tr, err := p.Load(s, transcript.Load{Window: win, Subagents: true, Branches: true})
				if err != nil {
					t.Fatal(err)
				}
				for _, e := range tr.Events {
					where := s.ID[:8] + "#" + itoa(e.Index) + " " + string(e.Kind)
					checkPos(t, where, "event", e.At, win, content)
					if e.Source != e.At.Path() || e.Line != e.At.N() {
						t.Errorf("%s %+v: source_file %q line %d, position %s:%d", where, win, e.Source, e.Line, e.At.Path(), e.At.N())
					}
					seen[p.Name()]++
					if len(s.Files) > 1 {
						seen["two files"]++
					}
					if strings.HasSuffix(e.At.Path(), jsonl.Compressed) {
						seen["compressed"]++
					}
					if win.TailBytes > 0 {
						seen["tail"]++
					}
				}
			}
		}
	}
	t.Logf("events checked: %v", seen)
	for _, want := range []string{"claude", "codex", "copilot", "two files", "compressed", "tail"} {
		if seen[want] == 0 {
			t.Errorf("no %s event was checked", want)
		}
	}
}

// checkPos checks that p is the start of a whole line in its log, with the line number the newlines before it give.
func checkPos(t *testing.T, where, what string, p jsonl.Pos, win jsonl.Window, content func(string) string) {
	t.Helper()
	if p.Path() == "" {
		t.Errorf("%s %+v: the %s has no position", where, win, what)
		return
	}
	src, off := content(p.Path()), p.Off()
	if off < 0 || off >= int64(len(src)) || off > 0 && src[off-1] != '\n' || strings.TrimSpace(src[off:off+1]) == "" {
		t.Errorf("%s %+v: the %s offset %d is not the start of a record in %s", where, win, what, off, filepath.Base(p.Path()))
		return
	}
	want := 1 + strings.Count(src[:off], "\n")
	if win.TailBytes > 0 {
		want = 0
	}
	if p.N() != want {
		t.Errorf("%s %+v: the %s at offset %d has line %d, want %d", where, win, what, off, p.N(), want)
	}
}

// positionCases adds the two log layouts that no world session has.
// The Codex session is also kept compressed, as Codex keeps a rollout after a week.
// The checkout log is copied into a second project folder with one more prompt, as a session carried on after a move.
func (w *world) positionCases() {
	w.t.Helper()
	rollouts, err := filepath.Glob(filepath.Join(w.root, "codex", "sessions", "*", "*", "*", "rollout-*-"+codexID+".jsonl"))
	if err != nil || len(rollouts) != 1 {
		w.t.Fatalf("codex rollout: %v %v", rollouts, err)
	}
	b, err := os.ReadFile(rollouts[0])
	if err != nil {
		w.t.Fatal(err)
	}
	if err := os.Remove(rollouts[0]); err != nil {
		w.t.Fatal(err)
	}
	w.write(rollouts[0]+jsonl.Compressed, 30*time.Minute, string(zstdtest.Frame(b)))

	logs, err := filepath.Glob(filepath.Join(w.root, "claude", "projects", "*", checkoutID+".jsonl"))
	if err != nil || len(logs) != 1 {
		w.t.Fatalf("checkout log: %v %v", logs, err)
	}
	b, err = os.ReadFile(logs[0])
	if err != nil {
		w.t.Fatal(err)
	}
	i := strings.LastIndex(string(b), `"uuid":"`) + len(`"uuid":"`)
	more := `{"type":"user","uuid":"5a1e0000-0000-4000-8000-0000000000ff","parentUuid":"` + string(b[i:i+36]) + `","sessionId":"` + checkoutID + `","cwd":"/work/shop2","timestamp":"` + w.at(time.Minute) + `","message":{"role":"user","content":"and the refund path"}}` + "\n"
	w.write(filepath.Join(w.root, "claude", "projects", "-work-shop2", checkoutID+".jsonl"), time.Minute, "\n"+string(b)+more)

	// Codex can log the item before the legacy record of the same answer, so the merge takes its other branch.
	id := "01a0aaaa-0000-7000-8000-0000000000aa"
	rec := func(typ string, payload map[string]any) map[string]any {
		return map[string]any{"timestamp": w.at(20 * time.Minute), "type": typ, "payload": payload}
	}
	day := w.now.Add(-20 * time.Minute)
	w.file(filepath.Join(w.root, "codex", "sessions", day.Format("2006"), day.Format("01"), day.Format("02"), "rollout-"+day.Format("2006-01-02T15-04-05")+"-"+id+".jsonl"), 20*time.Minute, []map[string]any{
		rec("session_meta", map[string]any{"id": id, "timestamp": w.at(20 * time.Minute), "cwd": "/work/shop", "originator": "codex_cli_rs", "cli_version": "0.130.0"}),
		rec("event_msg", map[string]any{"type": "item_completed", "thread_id": id, "item": map[string]any{"type": "AgentMessage", "phase": "final_answer",
			"content": []map[string]any{{"type": "Text", "text": "The refund path is covered."}}}}),
		rec("response_item", map[string]any{"type": "message", "role": "assistant", "phase": "final_answer", "content": []map[string]any{{"type": "output_text", "text": "The refund path is covered."}}}),
	})
}
