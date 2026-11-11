package diarize

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"memos2txt/internal/schema"
)

//go:embed scripts/pyannote_diarize.py
var scriptFS embed.FS

type Options struct {
	Model       string
	Device      string // auto | cpu | mps | cuda
	HFTOKEN     string
	NumSpeakers int
	MinSpeakers int
	MaxSpeakers int
	Verbose     bool
	CacheDir    string
}

type Result struct {
	Model        string
	Device       string
	SpeakerCount int
	Turns        []Turn
	Seconds      float64
	Speakers     []SpeakerSummary
}

type pyannoteOutput struct {
	OK           bool    `json:"ok"`
	Model        string  `json:"model"`
	Device       string  `json:"device"`
	SpeakerCount int     `json:"speaker_count"`
	Seconds      float64 `json:"seconds"`
	Turns        []struct {
		Start   float64 `json:"start"`
		End     float64 `json:"end"`
		Speaker string  `json:"speaker"`
	} `json:"turns"`
	Speakers []struct {
		ID            string  `json:"id"`
		Turns         int     `json:"turns"`
		SpeechSeconds float64 `json:"speech_seconds"`
	} `json:"speakers"`
	Error string `json:"error"`
}

// Run executes pyannote speaker diarization on a 16 kHz mono WAV file.
func Run(ctx context.Context, audioPath string, opts Options) (*Result, error) {
	if strings.TrimSpace(opts.Model) == "" {
		opts.Model = DefaultModel
	}
	if strings.TrimSpace(opts.Device) == "" {
		opts.Device = "auto"
	}

	st, err := EnsureInstalled(opts.CacheDir)
	if err != nil {
		return nil, err
	}
	if opts.CacheDir == "" {
		opts.CacheDir, _ = DefaultCacheDir()
	}

	scriptPath := filepath.Join(st.VenvPath, "pyannote_diarize.py")
	if err := writeEmbeddedScript(scriptPath); err != nil {
		return nil, schemaError(schema.ErrDiarizationFailed, "Failed to prepare diarization helper.", err.Error())
	}

	outPath := filepath.Join(st.VenvPath, fmt.Sprintf(".diarize-%d-%d.json", os.Getpid(), time.Now().UnixNano()))
	defer os.Remove(outPath)

	args := []string{
		scriptPath,
		"--audio", audioPath,
		"--model", opts.Model,
		"--output", outPath,
		"--device", opts.Device,
	}
	if opts.NumSpeakers > 0 {
		args = append(args, "--num-speakers", fmt.Sprintf("%d", opts.NumSpeakers))
	}
	if opts.MinSpeakers > 0 {
		args = append(args, "--min-speakers", fmt.Sprintf("%d", opts.MinSpeakers))
	}
	if opts.MaxSpeakers > 0 {
		args = append(args, "--max-speakers", fmt.Sprintf("%d", opts.MaxSpeakers))
	}

	cmd := exec.CommandContext(ctx, st.Python, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	if opts.Verbose {
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	} else {
		cmd.Stderr = &stderr
	}

	cmd.Env = append(os.Environ(),
		"HF_TOKEN="+opts.HFTOKEN,
		"HF_HUB_DISABLE_TELEMETRY=1",
		"TOKENIZERS_PARALLELISM=false",
		"PYTHONUNBUFFERED=1",
		"PYTORCH_ENABLE_MPS_FALLBACK=1",
	)

	runErr := cmd.Run()
	if runErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, schemaError(schema.ErrTimeout, "Diarization timed out.", ctx.Err().Error())
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail == "" {
			detail = runErr.Error()
		}
		if len(detail) > 6000 {
			detail = detail[len(detail)-6000:]
		}
		lower := strings.ToLower(detail)
		switch {
		case strings.Contains(lower, "401") || strings.Contains(lower, "403") ||
			strings.Contains(lower, "gated") || strings.Contains(lower, "access to model") ||
			strings.Contains(lower, "unauthorized") || strings.Contains(lower, "forbidden"):
			return nil, schemaError(schema.ErrAPIKeyMissing,
				"Hugging Face access denied for the diarization model.",
				"Accept the model terms at https://huggingface.co/"+opts.Model+
					" and store a token with: echo \"$HF_TOKEN\" | memos2txt auth huggingface --api-key-stdin. Details: "+detail)
		case strings.Contains(lower, "out of memory") || strings.Contains(lower, "mps backend"):
			return nil, schemaError(schema.ErrDiarizationFailed,
				"Diarization failed on the selected device.",
				"Retry with --diarization-device cpu. Details: "+detail)
		default:
			return nil, schemaError(schema.ErrDiarizationFailed, "Speaker diarization failed.", detail)
		}
	}

	b, err := os.ReadFile(outPath)
	if err != nil {
		return nil, schemaError(schema.ErrDiarizationFailed, "Diarizer did not produce output.", err.Error())
	}
	var parsed pyannoteOutput
	if err := json.Unmarshal(b, &parsed); err != nil {
		return nil, schemaError(schema.ErrDiarizationFailed, "Failed to parse diarizer output.", err.Error())
	}
	if !parsed.OK {
		msg := parsed.Error
		if msg == "" {
			msg = "unknown error"
		}
		return nil, schemaError(schema.ErrDiarizationFailed, "Speaker diarization failed.", msg)
	}

	res := &Result{
		Model:        parsed.Model,
		Device:       parsed.Device,
		SpeakerCount: parsed.SpeakerCount,
		Seconds:      parsed.Seconds,
	}
	for _, t := range parsed.Turns {
		res.Turns = append(res.Turns, Turn{Start: t.Start, End: t.End, Speaker: t.Speaker})
	}
	for _, s := range parsed.Speakers {
		res.Speakers = append(res.Speakers, SpeakerSummary{
			ID:            s.ID,
			Label:         speakerLabel(s.ID),
			Turns:         s.Turns,
			SpeechSeconds: s.SpeechSeconds,
		})
	}
	if res.SpeakerCount == 0 {
		res.SpeakerCount = len(res.Speakers)
	}
	return res, nil
}

func speakerLabel(id string) string {
	parts := strings.Split(strings.TrimSpace(id), "_")
	if len(parts) > 0 {
		if n, err := parseSmallInt(parts[len(parts)-1]); err == nil {
			return fmt.Sprintf("Speaker %d", n+1)
		}
	}
	return id
}

func parseSmallInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func writeEmbeddedScript(path string) error {
	b, err := scriptFS.ReadFile("scripts/pyannote_diarize.py")
	if err != nil {
		return err
	}
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, b) {
		return nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
