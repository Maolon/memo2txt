package providers

import (
	"context"
	"os/exec"
	"testing"
)

func TestProbeDurationSeconds(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	audio := writeTempFile(t, "sine.wav", "")
	if err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3", audio).Run(); err != nil {
		t.Fatalf("ffmpeg gen: %v", err)
	}
	d, err := ProbeDurationSeconds(context.Background(), audio)
	if err != nil {
		t.Fatalf("ProbeDurationSeconds: %v", err)
	}
	if d < 2.9 || d > 3.1 {
		t.Fatalf("duration=%f, want ~3.0", d)
	}
}
