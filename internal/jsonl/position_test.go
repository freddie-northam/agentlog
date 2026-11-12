package jsonl

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ryheff24/agentlog/internal/zstd/zstdtest"
)

// A position is how an event leads back to its record, so every line a read hands over must sit exactly where its Pos says.
// The checks recount from the raw bytes, never from Read: the offset is a line start, the line there is the line handed over, and the line number counts the newlines before it.
// Each log is read whole and through head and tail windows, plain and compressed, and the two must agree.
func TestPositionsLeadBackToTheLine(t *testing.T) {
	var many strings.Builder
	for i := range 3000 {
		fmt.Fprintf(&many, "{\"i\":%d,\"pad\":%q}\n", i, strings.Repeat("y", i%97))
		if i%13 == 0 {
			many.WriteString("\n  \n")
		}
	}
	big := `{"big":"` + strings.Repeat("x", 70<<10) + `"}`
	logs := map[string]string{
		"plain":              "{\"a\":1}\n{\"b\":2}\n{\"c\":3}\n",
		"blank and padded":   "{\"a\":1}\n\n  \n  {\"b\":2}  \n\t{\"c\":3}\n",
		"crlf":               "{\"a\":1}\r\n{\"b\":2}\r\n{\"c\":3}\r\n",
		"no final newline":   "{\"a\":1}\n{\"b\":2}\n{\"c\":",
		"past the buffer":    big + "\n{\"next\":1}\n" + big + "\n{\"last\":1}\n",
		"ring buffer turns":  many.String(),
		"blank first line":   "\n{\"a\":1}\n",
		"one line, no break": "{\"a\":1}",
	}
	for name, src := range logs {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			plain, zst := filepath.Join(dir, "log.jsonl"), filepath.Join(dir, "log.jsonl.zst")
			if err := os.WriteFile(plain, []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(zst, zstdtest.Frame([]byte(src)), 0o644); err != nil {
				t.Fatal(err)
			}
			size := int64(len(src))
			windows := []Window{{}, {HeadBytes: 1 << 30, TailBytes: size / 2}}
			for _, w := range []int64{1, 7, 8, 9, 13, 100, 4 << 10, 64 << 10, size/2 + 1, size - 1, size, size + 1} {
				windows = append(windows, Window{HeadBytes: w}, Window{TailBytes: w})
			}
			for _, w := range windows {
				want := positions(t, src, plain, w)
				got := positions(t, src, zst, w)
				if fmt.Sprint(got) != fmt.Sprint(want) {
					t.Errorf("%+v: compressed positions %v, plain %v", w, got, want)
				}
			}
		})
	}
}

// positions reads path through w, checks each Pos against src, and returns each as "n@off".
func positions(t *testing.T, src, path string, w Window) []string {
	t.Helper()
	var got []string
	last := int64(-1)
	err := Read(path, w, func(l Line) error {
		off := l.Off()
		switch {
		case l.Path() != path:
			t.Errorf("%+v: Path() = %q, want %q", w, l.Path(), path)
		case off <= last || off >= int64(len(src)):
			t.Errorf("%+v: offset %d after %d in a log of %d bytes", w, off, last, len(src))
		case off > 0 && src[off-1] != '\n':
			t.Errorf("%+v: offset %d is not a line start", w, off)
		default:
			end := strings.IndexByte(src[off:], '\n')
			if end < 0 {
				end = len(src) - int(off)
			}
			if at := bytes.TrimSpace([]byte(src[off : off+int64(end)])); !bytes.Equal(at, l.Bytes) {
				t.Errorf("%+v: offset %d holds %.40q, handed over %.40q", w, off, at, l.Bytes)
			}
			want := 1 + strings.Count(src[:off], "\n")
			if w.TailBytes > 0 {
				want = 0
			}
			if l.N() != want {
				t.Errorf("%+v: line at offset %d numbered %d, want %d", w, off, l.N(), want)
			}
		}
		last = off
		got = append(got, fmt.Sprintf("%d@%d", l.N(), off))
		return nil
	})
	if err != nil {
		t.Fatalf("%+v: %v", w, err)
	}
	return got
}

// A window with both fields set reads the tail, as the readers decided before Window existed.
func TestTailWinsOverHead(t *testing.T) {
	path := write(t, "{\"a\":1}\n{\"b\":2}\n{\"c\":3}\n")
	got, err := seen(path, Window{HeadBytes: 8, TailBytes: 8})
	if err != nil || strings.Join(got, " ") != `0:{"c":3}` {
		t.Errorf("Read() = %q, %v, want only the last line", got, err)
	}
}
