package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_PrunesToMaxEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	st, err := Open(path, 100)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	for i := 0; i < 120; i++ {
		key := fmt.Sprintf("k%d", i)
		if err := st.MarkSkipped(key, "https://example.com/"+key, "t"+key, "reason"); err != nil {
			t.Fatalf("MarkSkipped %s: %v", key, err)
		}
		time.Sleep(1 * time.Millisecond)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var m map[string]Entry
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(m) != 100 {
		t.Fatalf("expected 100 entries, got %d", len(m))
	}
	if _, ok := m["k119"]; !ok {
		t.Fatalf("expected newest key k119 to exist")
	}
	if _, ok := m["k0"]; ok {
		t.Fatalf("expected oldest key k0 to be pruned")
	}
}
