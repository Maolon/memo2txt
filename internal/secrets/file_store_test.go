package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileStoreSetGetDelete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	s, err := newFileStore("memos2txt")
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Set("GROQ_API_KEY", "k"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("GROQ_API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if got != "k" {
		t.Fatalf("got=%q", got)
	}

	if err := s.Delete("GROQ_API_KEY"); err != nil {
		t.Fatal(err)
	}
	got, err = s.Get("GROQ_API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("expected empty got=%q", got)
	}

	cfgPath := filepath.Join(home, ".memos2txt", "config.json")
	st, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("perm=%o", st.Mode().Perm())
	}
}
