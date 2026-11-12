// Package jsonl reads newline-delimited JSON logs whose lines can be many
// megabytes long, including cheap head and tail windows for discovery.
// A log whose name ends in .zst is read decompressed, wherever it is read.
package jsonl

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Ryheff24/agentlog/internal/zstd"
)

// ErrStop can be returned by a callback to end iteration early without error.
var ErrStop = errors.New("stop")

// Compressed is the suffix of a log stored zstd-compressed. Codex compresses
// its rollouts after about a week, so most of its history has it.
const Compressed = ".zst"

// IsLog reports whether name is a JSONL log, plain or compressed.
func IsLog(name string) bool {
	return strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".jsonl"+Compressed)
}

// TrimLog removes the .jsonl or .jsonl.zst suffix from name.
func TrimLog(name string) string {
	return strings.TrimSuffix(strings.TrimSuffix(name, Compressed), ".jsonl")
}

// Open opens a log for reading its records, decompressing it when its name
// says it is compressed.
func Open(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(path, Compressed) {
		return f, nil
	}
	return decompressed{zstd.NewReader(f), f}, nil
}

type decompressed struct {
	*zstd.Reader
	f *os.File
}

func (d decompressed) Close() error { return d.f.Close() }

// Pos is where one line sits in a log.
// Only this package builds one, so its offset is always the start of a line that was read.
// Pos is comparable, so a caller can find a record by it.
type Pos struct {
	path string
	n    int
	off  int64
}

// Path is the log file, as given to Read. It is empty only in the zero Pos.
func (p Pos) Path() string { return p.path }

// N is the 1-based line number, or 0 when a tail window cannot know it.
func (p Pos) N() int { return p.n }

// Off is the offset of the line's first byte in the decompressed log.
func (p Pos) Off() int64 { return p.off }

// Line is one line of a log and its position.
type Line struct {
	Pos
	Bytes []byte // trimmed, and valid only during the call
}

// Window selects a part of a log. The zero value selects all of it.
// When both fields are set, TailBytes wins.
type Window struct {
	HeadBytes int64 // read only the first HeadBytes
	TailBytes int64 // read only the last TailBytes
}

// Whole reports whether the window covers the entire log.
func (w Window) Whole() bool { return w.HeadBytes <= 0 && w.TailBytes <= 0 }

// Read calls fn for each non-empty line that the window selects.
//
// A head window leaves out a line it cut in two: it is not a record, and handing it over would have every windowed read of a large log report one it could not decode.
// A last line with no newline where the file itself ends is still handed over, in every window.
// A tail window hands over only whole lines, and their line numbers are 0.
func Read(path string, w Window, fn func(Line) error) error {
	switch {
	case w.TailBytes > 0:
		return tail(path, w.TailBytes, fn)
	case w.HeadBytes > 0:
		return head(path, w.HeadBytes, fn)
	}
	f, err := Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return each(f, Pos{path, 1, 0}, fn, nil)
}

func head(path string, maxBytes int64, fn func(Line) error) error {
	f, err := Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	lr := &io.LimitedReader{R: f, N: maxBytes}
	cut := func() bool {
		if lr.N > 0 {
			return false // the file ended inside the window
		}
		var b [1]byte
		n, _ := f.Read(b[:])
		return n > 0
	}
	return each(lr, Pos{path, 1, 0}, fn, cut)
}

func tail(path string, maxBytes int64, fn func(Line) error) error {
	if strings.HasSuffix(path, Compressed) {
		return tailStream(path, maxBytes, fn)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	off := st.Size() - maxBytes
	skipPartial := off > 0
	if skipPartial {
		// Start one byte early: if that byte ends a line, the window opens on
		// a whole line, and skipping "the partial first line" skips only it.
		off--
	} else {
		off = 0
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return fmt.Errorf("seek tail: %w", err)
	}
	r := bufio.NewReaderSize(f, 1<<16)
	if skipPartial {
		skipped, err := r.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil // window is inside one giant line
			}
			return err
		}
		off += int64(len(skipped))
	}
	return each(r, Pos{path, 0, off}, fn, nil)
}

// tailStream is tail for a log that can only be read from its start, as a compressed one can.
// It reads all of it and keeps the window that tail would have seeked to: the last maxBytes and the byte before them.
func tailStream(path string, maxBytes int64, fn func(Line) error) error {
	f, err := Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	window := int(maxBytes) + 1
	var keep []byte
	var total int64
	buf := make([]byte, 1<<16)
	for {
		n, err := f.Read(buf)
		total += int64(n)
		keep = append(keep, buf[:n]...)
		if len(keep) > 2*window {
			keep = keep[:copy(keep, keep[len(keep)-window:])]
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	skipPartial := total > maxBytes
	if len(keep) > window {
		keep = keep[len(keep)-window:]
	}
	off := total - int64(len(keep))
	r := bufio.NewReaderSize(bytes.NewReader(keep), 1<<16)
	if skipPartial {
		skipped, err := r.ReadBytes('\n')
		if err != nil {
			return nil // the window is inside one giant line
		}
		off += int64(len(skipped))
	}
	return each(r, Pos{path, 0, off}, fn, nil)
}

// each hands fn every non-empty line of r, which starts at the line at.
// A line number of 0 in at means the numbers are unknown and stay 0.
// cut, when given, is asked about a final line with no newline: true means the reader was cut short there and the line is only part of one.
func each(r io.Reader, at Pos, fn func(Line) error, cut func() bool) error {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReaderSize(r, 1<<16)
	}
	for {
		line, err := br.ReadBytes('\n')
		if err == io.EOF && len(line) > 0 && cut != nil && cut() {
			return nil
		}
		if len(line) > 0 {
			trimmed := bytes.TrimSpace(line)
			// A final line without '\n' may be a partial write; json decoding
			// in the callback rejects it, which is fine.
			if len(trimmed) > 0 {
				if cbErr := fn(Line{at, trimmed}); cbErr != nil {
					if errors.Is(cbErr, ErrStop) {
						return nil
					}
					return cbErr
				}
			}
			if at.n > 0 {
				at.n++
			}
			at.off += int64(len(line))
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
