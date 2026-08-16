package cache

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTranscriptPathDeterministic(t *testing.T) {
	s := NewTranscriptStore(filepath.Join(t.TempDir(), "transcripts"))
	k := Key{
		SHA256:      strings.Repeat("a", 64),
		Provider:    "groq",
		Model:       "whisper-large-v3",
		Language:    "auto",
		Diarization: false,
		Punctuate:   true,
	}
	p1 := s.TranscriptPath(k)
	p2 := s.TranscriptPath(k)
	if p1 != p2 {
		t.Fatalf("paths differ: %q vs %q", p1, p2)
	}
	if !strings.HasSuffix(p1, ".txt") {
		t.Fatalf("expected .txt: %q", p1)
	}
}
