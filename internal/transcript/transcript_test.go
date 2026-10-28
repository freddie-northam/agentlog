package transcript

import "testing"

func TestSkipsOrdersUnknownFirst(t *testing.T) {
	tr := &Transcript{}
	tr.Ignore("mode")
	tr.Ignore("mode")
	tr.Ignore("cost-state")
	tr.Unknown("brand-new")
	tr.Unknown("brand-new")
	tr.Unknown("other-new")

	var got []string
	for _, s := range tr.Skips() {
		got = append(got, s.Type)
	}
	// counts descending; at equal counts an unknown type sorts first because
	// it is the one worth acting on.
	want := []string{"brand-new", "mode", "other-new", "cost-state"}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("Skips() = %v, want %v", got, want)
		}
	}
	if tr.Ignored["mode"] != 2 || tr.Unknowns["brand-new"] != 2 {
		t.Errorf("counts = ignored %v unknown %v", tr.Ignored, tr.Unknowns)
	}
}

func TestHumanText(t *testing.T) {
	answers := []Answer{{Header: "Scope", Choice: "Only the parser"}, {Header: "Old flag", Choice: "Yes, keep it for one release", Notes: "keep it small"}}
	for _, tc := range []struct {
		name string
		e    Event
		want string
	}{
		{"human prompt", Event{Human: true, Kind: KindUser, Text: "fix it"}, "fix it"},
		{"non-human prompt", Event{Kind: KindUser, Text: "<agent-message>"}, ""},
		{"human answers", Event{Human: true, Kind: KindTool, Answers: answers}, "Scope: Only the parser · Old flag: Yes, keep it for one release (notes: keep it small)"},
		{"answer without header", Event{Human: true, Kind: KindTool, Answers: []Answer{{Choice: "Yes"}}}, "Yes"},
		{"subagent answers", Event{Kind: KindTool, Answers: answers}, ""},
		{"tool without answers", Event{Kind: KindTool, Tool: "Bash"}, ""},
	} {
		if got := tc.e.HumanText(); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}
