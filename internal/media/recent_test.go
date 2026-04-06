package media

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFindMostRecentVideoInDirs(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	f1 := filepath.Join(d1, "a.mp4")
	f2 := filepath.Join(d2, "b.mov")
	if err := os.WriteFile(f1, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(f2, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, source, err := FindMostRecentVideoInDirs([]string{d1, d2}, []string{"mp4", "mov"})
	if err != nil {
		t.Fatal(err)
	}
	if path != f2 {
		t.Fatalf("expected most recent %s got %s", f2, path)
	}
	if source != d2 {
		t.Fatalf("expected source %s got %s", d2, source)
	}
}
