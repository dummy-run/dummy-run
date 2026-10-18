package cast

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRepositoryCastsLoad checks every persona shipped in casts/ loads and
// has only lower-case IDs, a device the engine knows and at least two scenarios.
func TestRepositoryCastsLoad(t *testing.T) {
	root := filepath.Join("..", "..", "casts")
	casts, err := os.ReadDir(root)
	if err != nil {
		t.Skip("no casts directory")
	}
	for _, c := range casts {
		if !c.IsDir() {
			continue
		}
		dir := filepath.Join(root, c.Name())
		ids, err := List(dir)
		if err != nil || len(ids) == 0 {
			t.Fatalf("%s: %v %v", dir, ids, err)
		}
		for _, id := range ids {
			p, err := Load(dir, id)
			if err != nil {
				t.Errorf("%s/%s: %v", c.Name(), id, err)
				continue
			}
			if len(p.Scenarios) < 2 {
				t.Errorf("%s/%s has %d scenarios, want at least 2", c.Name(), id, len(p.Scenarios))
			}
			for _, s := range p.Scenarios {
				if s.Budget != 0 && (s.Budget < 25 || s.Budget > 40) {
					t.Errorf("%s/%s/%s asks for %d steps", c.Name(), id, s.ID, s.Budget)
				}
			}
		}
	}
}
