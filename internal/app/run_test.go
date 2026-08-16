package app

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"memos2txt/internal/cache"
	"memos2txt/internal/providers"
)

type fakeTranscriber struct {
	paths []string
}

func (f *fakeTranscriber) Transcribe(_ context.Context, p string, _ providers.TranscribeOptions) (string, error) {
	f.paths = append(f.paths, p)
	return "x", nil
}

// TestChunkSecondsSkipsSingleShot guards the bug where --chunk-seconds was
// ignored and Deepgram sync /v1/listen was attempted first (returns empty
// results for >1h audio). Regression: if ChunkSeconds>0 still does single-shot,
// fake.paths will contain "normalized.mp3".
func TestChunkSecondsSkipsSingleShot(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)

	src := filepath.Join(t.TempDir(), "src.wav")
	if err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3", src).Run(); err != nil {
		t.Fatalf("ffmpeg gen: %v", err)
	}

	ft := &fakeTranscriber{}
	cfg := Config{ChunkSeconds: 1}
	if _, _, err := transcribeWithPreprocess(context.Background(), ft, cache.Key{}, src,
		providers.TranscribeOptions{}, cfg, "test"); err != nil {
		t.Fatalf("transcribeWithPreprocess: %v", err)
	}
	if len(ft.paths) < 2 {
		t.Fatalf("expected >=2 chunk calls, got %d (%v)", len(ft.paths), ft.paths)
	}
	for _, p := range ft.paths {
		if filepath.Base(p) == "normalized.mp3" {
			t.Fatalf("single-shot leaked: %s (ChunkSeconds>0 should skip it)", p)
		}
	}
}

// TestAutoModeShortAudioSkipsChunking: with --chunk-seconds 0 (auto) on a short
// clip, only single-shot transcription runs — no ffmpeg segment step.
func TestAutoModeShortAudioSkipsChunking(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)

	src := filepath.Join(t.TempDir(), "src.wav")
	if err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3", src).Run(); err != nil {
		t.Fatalf("ffmpeg gen: %v", err)
	}

	ft := &fakeTranscriber{}
	cfg := Config{ChunkSeconds: 0}
	if _, _, err := transcribeWithPreprocess(context.Background(), ft, cache.Key{}, src,
		providers.TranscribeOptions{}, cfg, "test"); err != nil {
		t.Fatalf("transcribeWithPreprocess: %v", err)
	}
	if len(ft.paths) != 1 || filepath.Base(ft.paths[0]) != "normalized.mp3" {
		t.Fatalf("auto+short should single-shot normalized.mp3, got %v", ft.paths)
	}
}

func TestRunAuthMode(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	t.Setenv("GROQ_API_KEY", "")
	t.Setenv("DEEPGRAM_API_KEY", "")
	t.Setenv("ASSEMBLYAI_API_KEY", "")
	ctx := context.Background()

	// 1. Set key via auth command
	resp, err := Run(ctx, Config{
		AuthMode: true,
		Provider: "groq",
		APIKey:   "gsk-test-key-12345",
		Store:    "file",
	})
	if err != nil {
		t.Fatalf("Run auth set error: %v", err)
	}
	if !resp.OK || resp.Mode != "auth" || resp.Provider != "groq" {
		t.Fatalf("unexpected auth set response: %+v", resp)
	}

	// 2. Query status list
	respList, err := Run(ctx, Config{
		AuthMode: true,
		AuthList: true,
		Store:    "file",
	})
	if err != nil {
		t.Fatalf("Run auth list error: %v", err)
	}
	if !respList.OK || respList.Setup == nil {
		t.Fatalf("unexpected auth list response: %+v", respList)
	}
	if status := respList.Setup.Status["groq"]; status != "configured (store: file)" {
		t.Errorf("expected groq status to be configured, got %q", status)
	}
	if status := respList.Setup.Status["deepgram"]; status != "not configured" {
		t.Errorf("expected deepgram status to be not configured, got %q", status)
	}

	// 3. Unset key
	respUnset, err := Run(ctx, Config{
		AuthMode:    true,
		Provider:    "groq",
		UnsetAPIKey: true,
		Store:       "file",
	})
	if err != nil {
		t.Fatalf("Run auth unset error: %v", err)
	}
	if !respUnset.OK {
		t.Fatalf("unexpected auth unset response: %+v", respUnset)
	}
}
