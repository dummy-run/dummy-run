package coverage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dummy-run/dummy-run/internal/record"
)

// Load reads run records from files and directories. A directory contributes its own *.json files,
// in name order; the per-run folders of screenshots below it are not entered. Records of schema
// version 1 and 2 are both accepted. The first unreadable or invalid file stops the load.
func Load(paths ...string) ([]*record.Record, error) {
	var out []*record.Record
	seen := map[string]bool{}
	for _, p := range paths {
		names, err := expand(p)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			rec, err := record.Read(n)
			if err != nil {
				return nil, err
			}
			if seen[rec.ID] {
				return nil, fmt.Errorf("%s: run %s appears twice", n, rec.ID)
			}
			seen[rec.ID] = true
			out = append(out, rec)
		}
	}
	return out, nil
}

func expand(p string) ([]string, error) {
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{p}, nil
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, filepath.Join(p, e.Name()))
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("%s: no .json run records", p)
	}
	return names, nil
}
