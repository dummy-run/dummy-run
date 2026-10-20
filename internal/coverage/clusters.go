package coverage

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ClusterFile is one reviewer's grouping of findings into problems. Each cluster lists
// "<run id>/<finding id>" keys. A finding that is in no cluster is a problem of its own.
type ClusterFile struct {
	Name     string
	Clusters [][]string
}

// ParseClusters reads {"clusters": [["run/finding", ...], ...]}. A key may appear once.
func ParseClusters(name string, data []byte) (*ClusterFile, error) {
	var raw struct {
		Clusters [][]string `json:"clusters"`
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	seen := map[string]bool{}
	for _, c := range raw.Clusters {
		for _, k := range c {
			if !strings.Contains(k, "/") {
				return nil, fmt.Errorf("%s: %q is not <run id>/<finding id>", name, k)
			}
			if seen[k] {
				return nil, fmt.Errorf("%s: %q is in two clusters", name, k)
			}
			seen[k] = true
		}
	}
	return &ClusterFile{Name: name, Clusters: raw.Clusters}, nil
}

// ReadClusters reads a cluster file from disk.
func ReadClusters(path string) (*ClusterFile, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the path comes from the command line on purpose
	if err != nil {
		return nil, err
	}
	return ParseClusters(path, data)
}

// labels gives a label per key; keys the file does not list get a label of their own.
func (c *ClusterFile) labels(keys []string) []int {
	of := map[string]int{}
	for i, cl := range c.Clusters {
		for _, k := range cl {
			of[k] = i
		}
	}
	next := len(c.Clusters)
	out := make([]int, len(keys))
	for i, k := range keys {
		if l, ok := of[k]; ok {
			out[i] = l
		} else {
			out[i] = next
			next++
		}
	}
	return out
}
