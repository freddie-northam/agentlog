package claude

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ryheff24/agentlog/internal/transcript"
)

func TestQueuedHumanAndSyntheticPrompts(t *testing.T) {
	notify := "<task-notification><status>done</status><summary>build</summary></task-notification>"
	peer := "<cross-session-message from-name=\"peer\">merge it</cross-session-message>"
	queue := func(text string) map[string]any {
		return map[string]any{"type": "queue-operation", "operation": "enqueue", "content": text}
	}
	attach := func(text string) map[string]any {
		return map[string]any{"type": "attachment", "attachment": map[string]any{"type": "queued_command", "prompt": text}}
	}
	tr := loadLog(t, transcript.Load{}, queue(notify), attach(notify), queue(peer), attach(peer), queue("human queued"), attach("human queued"), queue("unanswered human"), user("<system-reminder>context</system-reminder>\nactual human"))
	humans, queued, unanswered := 0, 0, 0
	for _, e := range tr.Events {
		if e.Human {
			humans++
			if e.Tag == transcript.TagQueued {
				queued++
			}
			if e.Tag == transcript.TagUnanswered {
				unanswered++
			}
		}
		if e.Kind == transcript.KindUser && !e.Human {
			t.Errorf("synthetic user leaked: %+v", e)
		}
	}
	if humans != 3 || queued != 1 || unanswered != 1 {
		t.Fatalf("human %d queued %d unanswered %d: %+v", humans, queued, unanswered, tr.Events)
	}
}

func TestRefusalDetailsBecomeError(t *testing.T) {
	for _, reason := range []bool{true, false} {
		r := assistant(map[string]any{"type": "text", "text": "I cannot do that."})
		m := r["message"].(map[string]any)
		if reason {
			m["stop_reason"] = "refusal"
		} else {
			m["stop_details"] = map[string]any{"type": "refusal", "category": "safety", "explanation": "blocked"}
		}
		tr := loadLog(t, transcript.Load{}, r)
		if len(tr.Events) != 1 || tr.Events[0].Tag != "refusal" || tr.Session.Ending != transcript.EndingError {
			t.Fatalf("%+v", tr)
		}
	}
}

// answeredLog is a log with one AskUserQuestion call and its answer.
// A parent id makes the session a subagent.
func answeredLog(t *testing.T, withResult bool, parent string) *transcript.Transcript {
	t.Helper()
	call := assistant(toolUse("toolu_q1", "AskUserQuestion", map[string]any{"questions": []map[string]any{
		{"question": "Which scope should this cover?", "header": "Scope"},
		{"question": "Keep the old flag?", "header": "Old flag"},
	}}))
	extra := map[string]any{}
	if withResult {
		extra["toolUseResult"] = map[string]any{
			"questions": []map[string]any{
				{"question": "Which scope should this cover?", "header": "Scope", "options": []map[string]any{{"label": "Only the parser"}}},
				{"question": "Keep the old flag?", "header": "Old flag"},
			},
			"answers": map[string]any{
				"Keep the old flag?":             "Yes, keep it for one release",
				"Which scope should this cover?": "Only the parser",
			},
			"annotations": map[string]any{
				"Keep the old flag?":             map[string]any{"notes": "keep it small"},
				"Which scope should this cover?": map[string]any{"preview": "MOCKUP-TEXT"},
			},
		}
	}
	path := filepath.Join(t.TempDir(), sessionID+".jsonl")
	writeJSONL(t, path, []map[string]any{call, toolResult("toolu_q1", "User answered", extra)})
	tr, err := (&Provider{}).Load(&transcript.Session{ID: sessionID, ParentID: parent, Files: []string{path}}, transcript.Load{})
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestQuestionAnswersAreHumanWords(t *testing.T) {
	tr := answeredLog(t, true, "")
	plain := answeredLog(t, false, "")
	if len(tr.Events) != len(plain.Events) {
		t.Fatalf("answers changed the event count: %d vs %d", len(tr.Events), len(plain.Events))
	}
	for i := range tr.Events {
		if tr.Events[i].Index != plain.Events[i].Index {
			t.Errorf("event %d index %d vs %d", i, tr.Events[i].Index, plain.Events[i].Index)
		}
	}
	var e transcript.Event
	for _, ev := range tr.Events {
		if len(ev.Answers) > 0 {
			e = ev
		}
	}
	if !e.Human || len(e.Answers) != 2 || e.Answers[0].Header != "Scope" || e.Answers[1].Notes != "keep it small" {
		t.Fatalf("answer event: %+v", e)
	}
	want := "Scope: Only the parser · Old flag: Yes, keep it for one release (notes: keep it small)"
	if got := e.HumanText(); got != want {
		t.Errorf("HumanText = %q, want %q", got, want)
	}
	if strings.Contains(e.HumanText(), "MOCKUP") {
		t.Error("preview leaked into human words")
	}
}

func TestSubagentAnswersAreNotHuman(t *testing.T) {
	tr := answeredLog(t, true, "parent-session")
	found := false
	for _, e := range tr.Events {
		if len(e.Answers) > 0 {
			found = true
			if e.Human || e.HumanText() != "" {
				t.Errorf("subagent answer counted as human: %+v", e)
			}
		}
	}
	if !found {
		t.Fatal("answers were not read")
	}
}
