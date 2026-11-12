package app

import (
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
