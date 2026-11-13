package catalog

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/Ryheff24/agentlog/internal/jsonl"
	"github.com/Ryheff24/agentlog/internal/transcript"
)

// FileKind is a group of log files that no discovered session reads.
type FileKind struct {
	Pattern string `json:"pattern"`
	Count   int    `json:"count"`
	Bytes   int64  `json:"bytes"`
	Sample  string `json:"sample"`
}

// Unread lists the log files under a harness's roots that no session of all time lists, grouped by kind, most files first.
// A file kind that no discovery rule knows is otherwise invisible: no session holds it, so no count of records can show it.
func (c *Catalog) Unread(name string) ([]FileKind, error) {
	p := c.provider(name)
	ss, err := p.Discover(transcript.Query{Now: c.Now})
	if err != nil {
		return nil, err
	}
	read := map[string]bool{}
	for _, s := range ss {
		for _, f := range s.Files {
			read[f] = true
		}
	}
	kinds := map[string]*FileKind{}
	for _, root := range p.Roots() {
		// A root that is missing or partly unreadable still has its other files counted.
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || read[path] || logExt(path) == "" {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			pattern := kindOf(filepath.ToSlash(rel))
			k := kinds[pattern]
			if k == nil {
				k = &FileKind{Pattern: pattern, Sample: path}
				kinds[pattern] = k
			}
			k.Count++
			if info, err := d.Info(); err == nil {
				k.Bytes += info.Size()
			}
			return nil
		})
	}
	var out []FileKind
	for _, k := range kinds {
		out = append(out, *k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Pattern < out[j].Pattern
	})
	return out, nil
}

// logExt is the log extension a name ends in, or "" for a file that is not a log.
func logExt(name string) string {
	for _, ext := range []string{".jsonl" + jsonl.Compressed, ".jsonl"} {
		if strings.HasSuffix(name, ext) {
			return ext
		}
	}
	return ""
}

// kindOf replaces the parts of a path that differ from file to file, so files of one kind share one pattern.
// The first folder is a project, a year or a session, so it is always a wildcard.
// A name with a digit in it is an id, except for a prefix of letters before the first '-' or '_', such as `agent-` or `wf_`.
func kindOf(rel string) string {
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		ext := ""
		if i == len(parts)-1 {
			ext = logExt(part)
		}
		stem := strings.TrimSuffix(part, ext)
		switch {
		case i == 0 && len(parts) > 1:
			parts[i] = "*"
		case strings.IndexFunc(stem, unicode.IsDigit) >= 0:
			id := "<id>"
			if cut := strings.IndexAny(stem, "-_"); cut > 0 && strings.IndexFunc(stem[:cut], unicode.IsDigit) < 0 {
				id = stem[:cut+1] + id
			}
			parts[i] = id + ext
		}
	}
	return strings.Join(parts, "/")
}
