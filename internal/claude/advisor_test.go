package claude

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ryheff24/agentlog/internal/transcript"
)

func advisorCall(usage any, fallback any) map[string]any {
	rec := assistant(map[string]any{"type": "server_tool_use", "id": "advisor-test", "name": "advisor", "input": map[string]any{}})
	if usage != nil {
		rec["message"].(map[string]any)["usage"] = usage
	}
	rec["advisorModel"] = fallback
	return rec
}

func advisorUsage() map[string]any {
	return map[string]any{"iterations": []map[string]any{
		{"type": "message", "model": "caller-test", "input_tokens": 9, "output_tokens": 8},
		{"type": "advisor_message", "model": "claude-test-advisor", "input_tokens": 1200, "output_tokens": 300},
		{"type": "message", "input_tokens": 7, "output_tokens": 6},
	}}
}

func advisorAnswer(content any) map[string]any {
	return assistant(map[string]any{"type": "advisor_tool_result", "tool_use_id": "advisor-test", "content": content})
}

func advisorEncrypted() map[string]any {
	return map[string]any{"type": "advisor_redacted_result", "encrypted_content": "ENCRYPTED-" + strings.Repeat("Q", 200)}
}

func assertAdvisorPrivate(t *testing.T, tr *transcript.Transcript, secret string) {
	t.Helper()
	for _, e := range tr.Events {
		for _, s := range []string{e.Text, e.Input, e.Output} {
			if strings.Contains(s, secret) {
				t.Error("secret reached event")
			}
		}
	}
	data, err := json.Marshal(tr)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) {
		t.Error("secret reached transcript JSON")
	}
}

func TestAdvisorCallAndAnswer(t *testing.T) {
	tr := loadLog(t, transcript.Load{}, user("go"), advisorCall(advisorUsage(), "fallback-test"), advisorAnswer(advisorEncrypted()))
	if len(tr.Events) != 2 {
		t.Fatalf("events = %d, want user and tool", len(tr.Events))
	}
	e := tr.Events[1]
	if e.Kind != transcript.KindTool || e.Tool != "advisor" || e.Pending {
		t.Errorf("kind %q tool %q pending %v", e.Kind, e.Tool, e.Pending)
	}
	for _, want := range []string{"claude-test-advisor", "1200", "300"} {
		if !strings.Contains(strings.ReplaceAll(e.Input, ",", ""), want) {
			t.Errorf("input %q missing %q", e.Input, want)
		}
	}
	if !strings.Contains(e.Output, "encrypted") {
		t.Errorf("output = %q", e.Output)
	}
	if e.ResultLine != 3 || e.ResultSource == "" {
		t.Error("missing answer source")
	}
}

func TestAdvisorPrivacy(t *testing.T) {
	tr := loadLog(t, transcript.Load{}, user("go"), advisorCall(advisorUsage(), nil), advisorAnswer(advisorEncrypted()))
	assertAdvisorPrivate(t, tr, "ENCRYPTED-")
}

func TestAdvisorKnownBlocks(t *testing.T) {
	for _, tc := range []struct {
		name string
		rec  map[string]any
		key  string
	}{
		{"call", advisorCall(advisorUsage(), nil), "assistant:server_tool_use"},
		{"answer", advisorAnswer(advisorEncrypted()), "assistant:advisor_tool_result"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := loadLog(t, transcript.Load{}, tc.rec)
			if tr.Unknowns[tc.key] != 0 {
				t.Errorf("unknown %s = %d", tc.key, tr.Unknowns[tc.key])
			}
		})
	}
}

func TestAdvisorOrphan(t *testing.T) {
	tr := loadLog(t, transcript.Load{}, advisorAnswer(advisorEncrypted()))
	if len(tr.Events) != 1 || tr.Unpaired != 1 {
		t.Fatalf("events %d unpaired %d", len(tr.Events), tr.Unpaired)
	}
	if !strings.Contains(tr.Events[0].Output, "encrypted") {
		t.Errorf("output = %q", tr.Events[0].Output)
	}
	assertAdvisorPrivate(t, tr, "ENCRYPTED-")
}

func TestAdvisorMissingUsage(t *testing.T) {
	for _, usage := range []any{nil, map[string]any{}, map[string]any{"iterations": []map[string]any{{"type": "message", "input_tokens": 55}}}, "unreadable"} {
		tr := loadLog(t, transcript.Load{}, advisorCall(usage, nil))
		if tr.Unknowns["assistant:server_tool_use"] != 0 {
			t.Error("server tool call must be known")
		}
		if len(tr.Events) != 1 {
			t.Fatalf("events = %d", len(tr.Events))
		}
		e := tr.Events[0]
		if e.Kind != transcript.KindTool || e.Tool != "advisor" || e.Input != "" {
			t.Errorf("kind %q tool %q input %q", e.Kind, e.Tool, e.Input)
		}
	}
}

func TestAdvisorOtherServerTool(t *testing.T) {
	call := assistant(map[string]any{"type": "server_tool_use", "id": "search-test", "name": "web_search", "input": map[string]any{}})
	tr := loadLog(t, transcript.Load{}, call, assistant(map[string]any{"type": "web_search_tool_result", "tool_use_id": "search-test"}))
	if len(tr.Events) != 2 || tr.Events[0].Kind != transcript.KindTool || tr.Events[0].Tool != "web_search" {
		t.Errorf("kinds = %q", kinds(tr))
	}
	if tr.Unknowns["assistant:server_tool_use"] != 0 {
		t.Error("server tool call must be known")
	}
	if tr.Unknowns["assistant:web_search_tool_result"] != 1 {
		t.Error("web search result must remain unknown")
	}
}

func TestAdvisorFailure(t *testing.T) {
	tr := loadLog(t, transcript.Load{}, advisorCall(nil, nil), advisorAnswer(map[string]any{"type": "advisor_tool_result_error", "error_code": "overloaded"}))
	if len(tr.Events) != 1 {
		t.Fatalf("events = %d", len(tr.Events))
	}
	e := tr.Events[0]
	if e.Pending || !e.Failed || !strings.Contains(e.Output, "overloaded") {
		t.Errorf("pending %v failed %v output %q", e.Pending, e.Failed, e.Output)
	}
}

func TestAdvisorFallbackModel(t *testing.T) {
	for _, fallback := range []any{"claude-test-advisor", nil} {
		tr := loadLog(t, transcript.Load{}, advisorCall(map[string]any{"input_tokens": 99}, fallback))
		if len(tr.Events) != 1 {
			t.Fatalf("events = %d", len(tr.Events))
		}
		want, _ := fallback.(string)
		if e := tr.Events[0]; e.Kind != transcript.KindTool || e.Tool != "advisor" || e.Input != want {
			t.Errorf("kind %q tool %q input %q, want %q", e.Kind, e.Tool, e.Input, want)
		}
	}
}

func TestAdvisorFutureAnswer(t *testing.T) {
	tr := loadLog(t, transcript.Load{}, advisorCall(nil, nil), advisorAnswer(map[string]any{"type": "advisor_future_result", "secret_payload": "FUTURE-SECRET"}))
	if len(tr.Events) != 1 {
		t.Fatalf("events = %d", len(tr.Events))
	}
	e := tr.Events[0]
	if e.Pending || !strings.Contains(e.Output, "advisor_future_result") || !strings.Contains(e.Output, "cannot read") {
		t.Errorf("pending %v output %q", e.Pending, e.Output)
	}
	assertAdvisorPrivate(t, tr, "FUTURE-SECRET")
}

func TestAdvisorAnswerLabels(t *testing.T) {
	for _, label := range []string{"", "bad-code", "bad code", "é", strings.Repeat("a", 41), "Code_123", strings.Repeat("a", 40)} {
		for _, failed := range []bool{false, true} {
			content := map[string]any{"type": label, "secret_payload": "LABEL-SECRET"}
			if failed {
				content["type"], content["error_code"] = "advisor_tool_result_error", label
			}
			tr := loadLog(t, transcript.Load{}, advisorAnswer(content))
			if len(tr.Events) != 1 || tr.Unpaired != 1 {
				t.Fatalf("events %d unpaired %d", len(tr.Events), tr.Unpaired)
			}
			e := tr.Events[0]
			valid := label == "Code_123" || label == strings.Repeat("a", 40)
			if label != "" && strings.Contains(e.Output, label) != valid {
				t.Errorf("label %q output %q", label, e.Output)
			}
			if e.Failed != failed {
				t.Errorf("failed = %v, want %v", e.Failed, failed)
			}
			assertAdvisorPrivate(t, tr, "LABEL-SECRET")
		}
	}
}

func TestAdvisorCallInput(t *testing.T) {
	tr := loadLog(t, transcript.Load{}, advisorCall(advisorUsage(), "fallback-test"))
	if len(tr.Events) != 1 {
		t.Fatalf("events = %d", len(tr.Events))
	}
	e := tr.Events[0]
	if e.Tool != "advisor" || !e.Pending || e.Input != "claude-test-advisor, 1200 tokens in, 300 out" {
		t.Errorf("tool %q pending %v input %q", e.Tool, e.Pending, e.Input)
	}
}
