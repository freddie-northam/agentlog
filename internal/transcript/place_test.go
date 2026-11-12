package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ryheff24/agentlog/internal/jsonl"
)

// A position is for agentlog to find a record, not for --json.
// A key for it would change every event of every --json output, so the encoded event must hold only the keys it held before positions existed.
func TestPositionAddsNoJSONKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var p jsonl.Pos
	if err := jsonl.Read(path, jsonl.Window{}, func(l jsonl.Line) error { p = l.Pos; return nil }); err != nil {
		t.Fatal(err)
	}
	e := Event{Kind: KindUser, Text: "x"}
	e.Place(p)
	e.Answer(p)
	got, _ := json.Marshal(e)
	want := `{"source_file":"` + path + `","result_line":1,"result_source_file":"` + path + `","i":0,"time":"0001-01-01T00:00:00Z","kind":"user","text":"x","line":1}`
	if string(got) != want {
		t.Errorf("event JSON = %s\nwant          %s", got, want)
	}
}
