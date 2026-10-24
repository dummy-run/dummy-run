package page

import (
	"strings"
	"testing"
)

func TestReadAloudKeepsTheReaderLocal(t *testing.T) {
	if strings.Contains(readAloudJS, "globalThis.dummyrunVSR") {
		t.Error("the reading still uses the page's global")
	}
	defer func() {
		if recover() == nil {
			t.Error("a bundle without the assignment should panic")
		}
	}()
	localReader("(()=>{})()")
}
