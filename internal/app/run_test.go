package app

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"memos2txt/internal/cache"
	"memos2txt/internal/providers"
	"memos2txt/internal/schema"
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

func TestRunAuthHuggingFace(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	t.Setenv("HF_TOKEN", "")
	t.Setenv("HUGGING_FACE_HUB_TOKEN", "")
	t.Setenv("HUGGINGFACE_TOKEN", "")
	ctx := context.Background()

	resp, err := Run(ctx, Config{
		AuthMode: true,
		Provider: "hf",
		APIKey:   "hf_test_token",
		Store:    "file",
	})
	if err != nil {
		t.Fatalf("auth hf: %v", err)
	}
	if !resp.OK || resp.Provider != "huggingface" || resp.Setup == nil || resp.Setup.EnvVar != "HF_TOKEN" {
		t.Fatalf("unexpected auth response: %+v", resp)
	}

	list, err := Run(ctx, Config{AuthMode: true, AuthList: true, Store: "file"})
	if err != nil {
		t.Fatalf("auth list: %v", err)
	}
	if got := list.Setup.Status["huggingface"]; got != "configured (store: file)" {
		t.Fatalf("huggingface status = %q", got)
	}
	if got := list.Setup.Status["mlx-whisper"]; got != "not required (local)" {
		t.Fatalf("mlx-whisper status = %q", got)
	}
}

func TestRunDiarizerStatus(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)

	resp, err := Run(context.Background(), Config{
		DiarizerCommand: true,
		DiarizerAction:  "status",
	})
	if err != nil {
		t.Fatalf("diarizer status: %v", err)
	}
	if !resp.OK || resp.Mode != "diarizer_setup" || resp.LocalEnv == nil {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.LocalEnv.Installed {
		t.Fatal("expected a fresh HOME to have no installed runtime")
	}
	if !strings.Contains(resp.LocalEnv.VenvPath, tempDir) {
		t.Fatalf("venv path = %q, want under %q", resp.LocalEnv.VenvPath, tempDir)
	}
}

func TestResolveDiarizerValidation(t *testing.T) {
	localSpec, ok := providers.SpecFor("mlx-whisper")
	if !ok {
		t.Fatal("missing mlx-whisper spec")
	}
	cloudSpec, ok := providers.SpecFor("groq")
	if !ok {
		t.Fatal("missing groq spec")
	}

	tests := []struct {
		name string
		cfg  Config
		spec providers.Spec
		want schema.ErrorCode
	}{
		{
			name: "local with pyannote is accepted",
			cfg:  Config{Diarizer: "pyannote"},
			spec: localSpec,
		},
		{
			name: "cloud with local diarizer is rejected",
			cfg:  Config{Diarizer: "pyannote"},
			spec: cloudSpec,
			want: schema.ErrInvalidArgs,
		},
		{
			name: "conflicting speaker counts are rejected",
			cfg:  Config{Diarizer: "pyannote", NumSpeakers: 2, MinSpeakers: 1},
			spec: localSpec,
			want: schema.ErrInvalidArgs,
		},
		{
			name: "invalid device is rejected",
			cfg:  Config{Diarizer: "pyannote", DiarizerDevice: "tpu"},
			spec: localSpec,
			want: schema.ErrInvalidArgs,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, resp := resolveDiarizer(tt.cfg, tt.spec)
			if tt.want == "" {
				if resp != nil {
					t.Fatalf("unexpected error: %+v", resp)
				}
				return
			}
			if resp == nil || resp.Error == nil || resp.Error.Code != tt.want {
				t.Fatalf("resp = %+v, want error %s", resp, tt.want)
			}
		})
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
