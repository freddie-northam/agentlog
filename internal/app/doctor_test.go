package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDoctorReportsCallsWithNoInputRead(t *testing.T) {
	for _, tc := range []struct {
		name  string
		block map[string]any
		blank bool
	}{
		{"input renamed", map[string]any{"type": "tool_use", "id": "c1", "name": "Bash", "arguments": map[string]any{"command": "go test ./..."}}, true},
		{"key inside input renamed", toolUse("c1", "Bash", map[string]any{"cmd": "go test ./..."}), true},
		{"edit path renamed", toolUse("c1", "Edit", map[string]any{"path": "a.go", "old_string": "a", "new_string": "b"}), true},
		{"intact call", toolUse("c1", "Bash", map[string]any{"command": "go test ./..."}), false},
		{"tool that takes no input", toolUse("c1", "EnterPlanMode", map[string]any{}), false},
		{"unknown block type, already listed as unread", map[string]any{"type": "server_tool_use", "id": "s1", "name": "advisor", "input": map[string]any{}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := bareWorld(t)
			l := w.claude(siblingID, "/work/drift", time.Hour)
			l.user("-", "run the tests")
			l.assistant("", "tool_use", tc.block)
			l.result("c1", "ok", false, nil)
			l.save(filepath.Join(w.claudeProject(l.cwd), siblingID+".jsonl"), time.Minute)

			out, code := w.run("/work/drift", "doctor", "claude")
			if code != 0 {
				t.Fatal(out)
			}
			reported := strings.Contains(out, "1 tool call with no input this build can read")
			if reported != tc.blank {
				t.Fatalf("reported = %v, want %v:\n%s", reported, tc.blank, out)
			}
			if tc.blank && !strings.Contains(out, "2.1.281 ×1") {
				t.Errorf("the report must name the version the calls came from:\n%s", out)
			}
			if tc.blank && !strings.Contains(out, "agentlog "+siblingID[:8]+"#") {
				t.Errorf("the report must give a ref that opens one such call:\n%s", out)
			}
		})
	}
}

func TestDoctorStillReportsWhenARecordHasNoType(t *testing.T) {
	w := bareWorld(t)
	l := w.claude(siblingID, "/work/drift", time.Hour)
	l.user("-", "run the tests")
	l.add("", map[string]any{"note": "a record the harness wrote with no type"})
	l.assistant("", "tool_use", map[string]any{"type": "tool_use", "id": "c1", "name": "Bash", "arguments": map[string]any{"command": "go test ./..."}})
	l.result("c1", "ok", false, nil)
	l.save(filepath.Join(w.claudeProject(l.cwd), siblingID+".jsonl"), time.Minute)

	out, _ := w.run("/work/drift", "doctor", "claude")
	if !strings.Contains(out, "1 tool call with no input this build can read") {
		t.Fatalf("a record with no type hid the empty call:\n%s", out)
	}
}

func TestDoctorLeavesOutATypeThePersonDeclared(t *testing.T) {
	w := bareWorld(t)
	dir := os.Getenv("AGENTLOG_CONFIG")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "types.json"), []byte(`{"claude":[{"type":"session-weather","role":"ignore"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	l := w.claude(siblingID, "/work/drift", time.Hour)
	l.user("-", "run the tests")
	l.add("", map[string]any{"type": "session-weather", "name": "probe", "content": "sunny"})
	l.assistant("", "tool_use", toolUse("c1", "Bash", map[string]any{"command": "go test ./..."}))
	l.result("c1", "ok", false, nil)
	l.save(filepath.Join(w.claudeProject(l.cwd), siblingID+".jsonl"), time.Minute)

	out, _ := w.run("/work/drift", "doctor", "claude")
	if strings.Contains(out, "with no input") {
		t.Fatalf("a declared type counted as an empty call:\n%s", out)
	}
}

func TestDoctorRefOpensTheEmptyCallPastARewoundTurn(t *testing.T) {
	w := bareWorld(t)
	l := w.claude(siblingID, "/work/drift", time.Hour)
	root := l.user("-", "start")
	l.assistant("", "end_turn", text("an answer that was rewound"))
	l.user(root, "run the tests")
	l.assistant("", "tool_use", map[string]any{"type": "tool_use", "id": "c1", "name": "Bash", "arguments": map[string]any{"command": "go test ./..."}})
	l.result("c1", "ok", false, nil)
	l.assistant("", "tool_use", toolUse("c2", "Bash", map[string]any{"command": "echo intact"}))
	l.result("c2", "intact", false, nil)
	l.save(filepath.Join(w.claudeProject(l.cwd), siblingID+".jsonl"), time.Minute)

	out, _ := w.run("/work/drift", "doctor", "claude")
	line := out[strings.Index(out, "with no input"):]
	line = line[:strings.Index(line, "\n")]
	ref := strings.Fields(line[strings.Index(line, "agentlog ")+len("agentlog "):])
	opened, code := w.run("/work/drift", ref...)
	if code != 0 || !strings.Contains(opened, "call: Bash") || strings.Contains(opened, "echo intact") {
		t.Fatalf("ref %v did not open the empty call:\n%s", ref, opened)
	}
}
