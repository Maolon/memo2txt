package providers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"memos2txt/internal/schema"
)

type PreprocessOptions struct {
	ChunkSeconds  int
	BitrateKbps   int
	SampleRateHz  int
	VerboseStderr bool
}

func NormalizeToMP3(ctx context.Context, inputPath, outputPath string, opts PreprocessOptions) error {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return ProviderError{
			Code:    schema.ErrInvalidArgs,
			Message: "ffmpeg not found (required for --auto-preprocess).",
			Detail:  "Install ffmpeg or disable --auto-preprocess.",
		}
	}

	if opts.BitrateKbps <= 0 {
		opts.BitrateKbps = 32
	}
	if opts.SampleRateHz <= 0 {
		opts.SampleRateHz = 16000
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
		return ProviderError{Code: schema.ErrProviderError, Message: "Failed to create temp dir.", Detail: err.Error()}
	}

	args := []string{
		"-hide_banner",
		"-y",
	}
	if !opts.VerboseStderr {
		args = append(args, "-loglevel", "error")
	}
	args = append(args,
		"-i", inputPath,
		"-vn",
		"-ac", "1",
		"-ar", fmt.Sprintf("%d", opts.SampleRateHz),
		"-b:a", fmt.Sprintf("%dk", opts.BitrateKbps),
		"-codec:a", "libmp3lame",
		outputPath,
	)

	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	// ffmpeg logs to stderr; keep stdout empty.
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return ProviderError{Code: schema.ErrProviderError, Message: "ffmpeg preprocess failed.", Detail: err.Error()}
	}
	return nil
}

func SegmentToChunks(ctx context.Context, inputPath, chunksDir string, opts PreprocessOptions) ([]string, error) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, ProviderError{
			Code:    schema.ErrInvalidArgs,
			Message: "ffmpeg not found (required for --auto-preprocess).",
			Detail:  "Install ffmpeg or disable --auto-preprocess.",
		}
	}
	if opts.ChunkSeconds <= 0 {
		return []string{inputPath}, nil
	}

	_ = os.RemoveAll(chunksDir)
	if err := os.MkdirAll(chunksDir, 0o700); err != nil {
		return nil, ProviderError{Code: schema.ErrProviderError, Message: "Failed to create chunk dir.", Detail: err.Error()}
	}

	pattern := filepath.Join(chunksDir, "chunk_%03d.mp3")
	args := []string{
		"-hide_banner",
		"-y",
	}
	if !opts.VerboseStderr {
		args = append(args, "-loglevel", "error")
	}
	args = append(args,
		"-i", inputPath,
		"-f", "segment",
		"-segment_time", fmt.Sprintf("%d", opts.ChunkSeconds),
		"-reset_timestamps", "1",
		pattern,
	)

	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, ProviderError{Code: schema.ErrProviderError, Message: "ffmpeg chunking failed.", Detail: err.Error()}
	}

	entries, err := os.ReadDir(chunksDir)
	if err != nil {
		return nil, ProviderError{Code: schema.ErrProviderError, Message: "Failed to read chunk dir.", Detail: err.Error()}
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "chunk_") && strings.HasSuffix(strings.ToLower(name), ".mp3") {
			out = append(out, filepath.Join(chunksDir, name))
		}
	}
	sort.Strings(out)
	return out, nil
}

func ProbeDurationSeconds(ctx context.Context, path string) (float64, error) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return 0, ProviderError{
			Code:    schema.ErrInvalidArgs,
			Message: "ffprobe not found.",
			Detail:  "Install ffmpeg (ships with ffprobe).",
		}
	}
	cmd := exec.CommandContext(ctx, ffprobe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, ProviderError{Code: schema.ErrProviderError, Message: "ffprobe failed.", Detail: err.Error()}
	}
	var d float64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &d); err != nil {
		return 0, ProviderError{Code: schema.ErrProviderError, Message: "ffprobe returned unparseable duration.", Detail: strings.TrimSpace(string(out))}
	}
	return d, nil
}

func IsRequestTooLarge(err error) bool {
	var pe ProviderError
	if errors.As(err, &pe) {
		d := strings.ToLower(pe.Detail)
		m := strings.ToLower(pe.Message)
		if strings.Contains(d, "status=413") || strings.Contains(d, "request_too_large") {
			return true
		}
		if strings.Contains(m, "too large") {
			return true
		}
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "status=413") || strings.Contains(s, "request_too_large") || strings.Contains(s, "entity too large")
}
