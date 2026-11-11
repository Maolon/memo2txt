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

func TestTranscriptPathSanitizesModelAndDiarizer(t *testing.T) {
	s := NewTranscriptStore(t.TempDir())
	k := Key{
		SHA256:        strings.Repeat("b", 64),
		Provider:      "mlx-whisper",
		Model:         "mlx-community/whisper-large-v3-turbo",
		Language:      "auto",
		Diarization:   true,
		Punctuate:     true,
		Diarizer:      "pyannote",
		DiarizerModel: "pyannote/speaker-diarization-community-1",
	}
	p := s.TranscriptPath(k)
	if filepath.Dir(p) != s.Dir {
		t.Fatalf("transcript path escaped store dir: %q", p)
	}
	if strings.Contains(filepath.Base(p), "/") {
		t.Fatalf("filename still contains slash: %q", p)
	}
	if !strings.Contains(filepath.Base(p), "pyannote-speaker-diarization-community-1") {
		t.Fatalf("diarizer suffix missing: %q", p)
	}
}

func TestTranscriptPathDiarizerChangesKey(t *testing.T) {
	s := NewTranscriptStore(t.TempDir())
	base := Key{SHA256: strings.Repeat("c", 64), Provider: "mlx-whisper", Model: "m", Language: "auto", Punctuate: true}
	withDiarizer := base
	withDiarizer.Diarization = true
	withDiarizer.Diarizer = "pyannote"
	withDiarizer.DiarizerModel = "model"
	if s.TranscriptPath(base) == s.TranscriptPath(withDiarizer) {
		t.Fatal("diarizer must change the cache key")
	}
}
