package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"memos2txt/internal/schema"
)

const (
	DefaultMLXWhisperModel = "mlx-community/whisper-large-v3-turbo"
	mlxWhisperBinary       = "mlx_whisper"
)

type MLXWhisperClientOptions struct {
	Model         string
	Binary        string
	VerboseStderr bool
}

type MLXWhisperClient struct {
	opts MLXWhisperClientOptions
}

func NewMLXWhisperClient(opts MLXWhisperClientOptions) *MLXWhisperClient {
	if strings.TrimSpace(opts.Model) == "" {
		opts.Model = DefaultMLXWhisperModel
	}
	if strings.TrimSpace(opts.Binary) == "" {
		opts.Binary = os.Getenv("MEMOS2TXT_MLX_WHISPER_BIN")
	}
	if strings.TrimSpace(opts.Binary) == "" {
		opts.Binary = mlxWhisperBinary
	}
	return &MLXWhisperClient{opts: opts}
}

// DetectMLXWhisperBinary resolves the local mlx_whisper executable. It returns
// a schema error suitable for a JSON response when the dependency is missing.
func DetectMLXWhisperBinary() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("MEMOS2TXT_MLX_WHISPER_BIN")); configured != "" {
		if _, err := os.Stat(configured); err != nil {
			return "", ProviderError{
				Code:    schema.ErrLocalDependencyMissing,
				Message: "mlx_whisper binary not found.",
				Detail:  "MEMOS2TXT_MLX_WHISPER_BIN=" + configured + " is not executable: " + err.Error(),
			}
		}
		return configured, nil
	}
	for _, name := range []string{"mlx_whisper", "mlx-whisper"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", ProviderError{
		Code:    schema.ErrLocalDependencyMissing,
		Message: "mlx_whisper is not installed.",
		Detail:  "Run: brew install mlx-whisper (Apple Silicon only), or set MEMOS2TXT_MLX_WHISPER_BIN.",
	}
}

func (c *MLXWhisperClient) Transcribe(ctx context.Context, audioPath string, opts TranscribeOptions) (string, error) {
	segments, err := c.TranscribeSegments(ctx, audioPath, opts)
	if err != nil {
		return "", err
	}
	return FormatPlainSegments(segments), nil
}

func (c *MLXWhisperClient) TranscribeSegments(ctx context.Context, audioPath string, opts TranscribeOptions) ([]Segment, error) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return nil, ProviderError{
			Code:    schema.ErrLocalDependencyMissing,
			Message: "mlx-whisper requires Apple Silicon macOS.",
			Detail:  fmt.Sprintf("current platform=%s/%s. Use a cloud provider on this platform.", runtime.GOOS, runtime.GOARCH),
		}
	}

	bin, err := DetectMLXWhisperBinary()
	if err != nil {
		return nil, err
	}

	outputDir, err := os.MkdirTemp("", "memos2txt-mlx-*")
	if err != nil {
		return nil, ProviderError{Code: schema.ErrProviderError, Message: "Failed to create temp dir.", Detail: err.Error()}
	}
	defer os.RemoveAll(outputDir)

	const outputName = "transcript"
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		segments, detail, retryable, err := c.runOnce(ctx, bin, audioPath, opts, outputDir, outputName)
		if err == nil {
			return segments, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
		// mlx-whisper 0.4.3 only looks for weights.safetensors/weights.npz.
		// Some 4bit/8bit MLX repos ship model.safetensors instead. Add a
		// compatibility alias in the HF snapshot and retry once.
		changed, aliasErr := ensureMLXWeightsAlias(c.opts.Model)
		if aliasErr != nil {
			return nil, ProviderError{
				Code:    schema.ErrProviderError,
				Message: "mlx_whisper model layout is incompatible.",
				Detail:  aliasErr.Error() + " | " + detail,
			}
		}
		if !changed {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *MLXWhisperClient) runOnce(
	ctx context.Context,
	bin string,
	audioPath string,
	opts TranscribeOptions,
	outputDir string,
	outputName string,
) ([]Segment, string, bool, error) {
	args := []string{
		audioPath,
		"--model", c.opts.Model,
		"--output-format", "json",
		"--output-dir", outputDir,
		"--output-name", outputName,
		"--verbose", "False",
		"--word-timestamps", "True",
		"--task", "transcribe",
	}
	if lang := strings.TrimSpace(opts.Language); lang != "" && !strings.EqualFold(lang, "auto") {
		args = append(args, "--language", lang)
	}
	if prompt := strings.TrimSpace(opts.InitialPrompt); prompt != "" {
		args = append(args, "--initial-prompt", prompt)
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	if c.opts.VerboseStderr {
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	} else {
		cmd.Stderr = &stderr
	}

	runErr := cmd.Run()
	if runErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, "", false, ProviderError{Code: schema.ErrTimeout, Message: "Timed out.", Detail: ctx.Err().Error()}
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, "", false, ProviderError{Code: schema.ErrTimeout, Message: "Canceled.", Detail: ctx.Err().Error()}
		}
		detail := commandDetail(stdout.String(), stderr.String())
		code := schema.ErrProviderError
		msg := "mlx_whisper failed."
		lower := strings.ToLower(detail)
		if strings.Contains(lower, "no such file") || strings.Contains(lower, "not found") {
			msg = "mlx_whisper could not load the audio or model."
		}
		return nil, detail, false, ProviderError{Code: code, Message: msg, Detail: detail}
	}

	jsonPath := filepath.Join(outputDir, outputName+".json")
	b, err := os.ReadFile(jsonPath)
	if err != nil {
		detail := commandDetail(stdout.String(), stderr.String())
		if detail == "" {
			detail = err.Error()
		} else {
			detail += " | " + err.Error()
		}
		return nil, detail, true, ProviderError{
			Code:    schema.ErrProviderError,
			Message: "mlx_whisper did not produce JSON output.",
			Detail:  detail,
		}
	}

	var parsed mlxWhisperOutput
	if err := json.Unmarshal(b, &parsed); err != nil {
		return nil, "", false, ProviderError{Code: schema.ErrProviderError, Message: "Failed to parse mlx_whisper JSON.", Detail: err.Error()}
	}
	segments, err := segmentsFromMLX(parsed)
	return segments, "", false, err
}

func commandDetail(stdout, stderr string) string {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		detail = strings.TrimSpace(stdout)
	}
	if len(detail) > 4000 {
		detail = detail[len(detail)-4000:]
	}
	return detail
}

// ensureMLXWeightsAlias adds a weights.safetensors symlink next to
// model.safetensors for mlx-whisper 0.4.3, which only understands the older
// weight filename. It returns true when at least one alias was created.
func ensureMLXWeightsAlias(model string) (bool, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return false, nil
	}
	if _, err := os.Stat(model); err == nil {
		return false, nil // local model directory
	}
	if strings.HasPrefix(model, ".") || strings.HasPrefix(model, "/") || strings.HasPrefix(model, "~") {
		return false, nil
	}
	repo := strings.SplitN(model, "@", 2)[0]
	dirName := "models--" + strings.ReplaceAll(repo, "/", "--")

	changed := false
	for _, hubDir := range hfHubCacheDirs() {
		snapshots := filepath.Join(hubDir, dirName, "snapshots")
		entries, err := os.ReadDir(snapshots)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dir := filepath.Join(snapshots, entry.Name())
			if _, err := os.Stat(filepath.Join(dir, "model.safetensors")); err != nil {
				continue
			}
			alias := filepath.Join(dir, "weights.safetensors")
			if _, err := os.Stat(alias); err == nil {
				continue
			}
			// Remove a broken symlink before recreating it.
			if _, err := os.Lstat(alias); err == nil {
				_ = os.Remove(alias)
			}
			if err := os.Symlink("model.safetensors", alias); err != nil {
				return changed, err
			}
			changed = true
		}
	}
	return changed, nil
}

func hfHubCacheDirs() []string {
	var dirs []string
	if v := strings.TrimSpace(os.Getenv("HF_HUB_CACHE")); v != "" {
		dirs = append(dirs, v)
	}
	if v := strings.TrimSpace(os.Getenv("HF_HOME")); v != "" {
		dirs = append(dirs, filepath.Join(v, "hub"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(home, ".cache", "huggingface", "hub"),
			filepath.Join(home, "Library", "Caches", "huggingface", "hub"),
		)
	}
	seen := make(map[string]bool, len(dirs))
	out := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		out = append(out, dir)
	}
	return out
}

func segmentsFromMLX(parsed mlxWhisperOutput) ([]Segment, error) {
	segments := make([]Segment, 0, len(parsed.Segments))
	for _, s := range parsed.Segments {
		text := strings.TrimSpace(s.Text)
		if text == "" {
			continue
		}
		seg := Segment{Start: s.Start, End: s.End, Text: text}
		for _, w := range s.Words {
			wt := strings.TrimSpace(w.Word)
			if wt == "" {
				continue
			}
			seg.Words = append(seg.Words, Word{Text: wt, Start: w.Start, End: w.End})
		}
		segments = append(segments, seg)
	}

	if len(segments) == 0 {
		text := strings.TrimSpace(parsed.Text)
		if text == "" {
			return nil, ProviderError{Code: schema.ErrProviderError, Message: "mlx_whisper returned an empty transcript.", Detail: ""}
		}
		return []Segment{{Start: 0, End: 0, Text: text}}, nil
	}
	return segments, nil
}

type mlxWhisperOutput struct {
	Text     string `json:"text"`
	Language string `json:"language"`
	Segments []struct {
		Start float64 `json:"start"`
		End   float64 `json:"end"`
		Text  string  `json:"text"`
		Words []struct {
			Word  string  `json:"word"`
			Start float64 `json:"start"`
			End   float64 `json:"end"`
		} `json:"words"`
	} `json:"segments"`
}

// FormatPlainSegments renders ASR segments without speaker labels.
func FormatPlainSegments(segments []Segment) string {
	var b strings.Builder
	for _, s := range segments {
		text := strings.TrimSpace(s.Text)
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(text)
	}
	return strings.TrimSpace(b.String())
}
