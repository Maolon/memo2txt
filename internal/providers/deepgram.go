package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"memos2txt/internal/schema"
)

type DeepgramClientOptions struct {
	Model         string
	Timeout       time.Duration
	BaseURL       string
	VerboseStderr bool
}

type DeepgramClient struct {
	apiKey string
	opts   DeepgramClientOptions
	http   *http.Client
}

func NewDeepgramClient(apiKey string, opts DeepgramClientOptions) *DeepgramClient {
	baseTimeout := opts.Timeout
	if baseTimeout <= 0 {
		baseTimeout = 300 * time.Second
	}
	if strings.TrimSpace(opts.BaseURL) == "" {
		opts.BaseURL = "https://api.deepgram.com"
	}
	if strings.TrimSpace(opts.Model) == "" {
		opts.Model = "nova-3"
	}
	return &DeepgramClient{
		apiKey: strings.TrimSpace(apiKey),
		opts:   opts,
		http: &http.Client{
			Timeout: baseTimeout,
		},
	}
}

type deepgramResponse struct {
	Results struct {
		Utterances []struct {
			Start      float64 `json:"start"`
			End        float64 `json:"end"`
			Transcript string  `json:"transcript"`
			Confidence float64 `json:"confidence"`
			Speaker    int     `json:"speaker"`
		} `json:"utterances"`
		Channels []struct {
			Alternatives []struct {
				Transcript string `json:"transcript"`
			} `json:"alternatives"`
		} `json:"channels"`
	} `json:"results"`
}

func (c *DeepgramClient) Transcribe(ctx context.Context, audioPath string, opts TranscribeOptions) (string, error) {
	if c.apiKey == "" {
		return "", ProviderError{Code: schema.ErrAPIKeyMissing, Message: "Missing API key.", Detail: "DEEPGRAM_API_KEY"}
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			return "", ProviderError{Code: schema.ErrTimeout, Message: "Timed out.", Detail: ctx.Err().Error()}
		}
		text, err := c.transcribeOnce(ctx, audioPath, opts)
		if err == nil {
			return text, nil
		}
		lastErr = err

		var pe ProviderError
		if errors.As(err, &pe) && pe.Code == schema.ErrProviderError {
			break
		}
		time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
	}
	return "", lastErr
}

func (c *DeepgramClient) transcribeOnce(ctx context.Context, audioPath string, opts TranscribeOptions) (string, error) {
	f, err := os.Open(audioPath)
	if err != nil {
		return "", ProviderError{Code: schema.ErrFileUnreadable, Message: "File unreadable.", Detail: err.Error()}
	}
	defer f.Close()

	u := strings.TrimRight(c.opts.BaseURL, "/") + "/v1/listen"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, f)
	if err != nil {
		return "", ProviderError{Code: schema.ErrUploadFailed, Message: "Failed to build request.", Detail: err.Error()}
	}

	q := req.URL.Query()
	q.Set("model", c.opts.Model)
	if opts.Punctuate {
		q.Set("punctuate", "true")
	}
	if opts.Diarization {
		q.Set("diarize", "true")
		q.Set("utterances", "true")
	}
	q.Set("smart_format", "true")
	if lang := strings.TrimSpace(opts.Language); lang != "" && strings.ToLower(lang) != "auto" {
		q.Set("language", lang)
	}
	req.URL.RawQuery = q.Encode()

	req.Header.Set("Authorization", "Token "+c.apiKey)
	req.Header.Set("Content-Type", contentTypeForPath(audioPath))

	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", ProviderError{Code: schema.ErrTimeout, Message: "Timed out.", Detail: err.Error()}
		}
		return "", ProviderError{Code: schema.ErrUploadFailed, Message: "Upload failed.", Detail: err.Error()}
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code := schema.ErrProviderError
		msg := "Provider error."
		if resp.StatusCode >= 500 {
			code = schema.ErrUploadFailed
			msg = "Provider unavailable."
		}
		if resp.StatusCode == 413 {
			code = schema.ErrInvalidArgs
			msg = "File too large for provider."
		}
		return "", ProviderError{
			Code:    code,
			Message: msg,
			Detail:  fmt.Sprintf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(b))),
		}
	}

	var dg deepgramResponse
	if err := json.Unmarshal(b, &dg); err != nil {
		return "", ProviderError{Code: schema.ErrProviderError, Message: "Failed to parse provider response.", Detail: err.Error()}
	}

	// Prefer utterances when present (needed for speaker labels).
	if len(dg.Results.Utterances) > 0 {
		var sb strings.Builder
		for _, u := range dg.Results.Utterances {
			t := strings.TrimSpace(u.Transcript)
			if t == "" {
				continue
			}
			sb.WriteString(fmt.Sprintf("[Speaker:%d] %s\n", u.Speaker, t))
		}
		out := strings.TrimSuffix(sb.String(), "\n")
		if strings.TrimSpace(out) == "" {
			return "", ProviderError{Code: schema.ErrProviderError, Message: "Provider response missing transcript.", Detail: "utterances present but empty"}
		}
		return out, nil
	}

	if len(dg.Results.Channels) == 0 || len(dg.Results.Channels[0].Alternatives) == 0 {
		return "", ProviderError{Code: schema.ErrProviderError, Message: "Provider response missing transcript.", Detail: ""}
	}
	return dg.Results.Channels[0].Alternatives[0].Transcript, nil
}

func contentTypeForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".m4a", ".mp4":
		return "audio/mp4"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	default:
		return "application/octet-stream"
	}
}
