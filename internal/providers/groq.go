package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"memos2txt/internal/schema"
)

type GroqClientOptions struct {
	Model         string
	Timeout       time.Duration
	BaseURL       string
	VerboseStderr bool
}

type GroqClient struct {
	apiKey string
	opts   GroqClientOptions
	http   *http.Client
}

func NewGroqClient(apiKey string, opts GroqClientOptions) *GroqClient {
	baseTimeout := opts.Timeout
	if baseTimeout <= 0 {
		baseTimeout = 300 * time.Second
	}
	if strings.TrimSpace(opts.BaseURL) == "" {
		opts.BaseURL = "https://api.groq.com"
	}
	if strings.TrimSpace(opts.Model) == "" {
		opts.Model = "whisper-large-v3"
	}
	return &GroqClient{
		apiKey: strings.TrimSpace(apiKey),
		opts:   opts,
		http: &http.Client{
			Timeout: baseTimeout,
		},
	}
}

type groqTranscribeResponse struct {
	Text string `json:"text"`
}

func (c *GroqClient) Transcribe(ctx context.Context, audioPath string, opts TranscribeOptions) (string, error) {
	if c.apiKey == "" {
		return "", ProviderError{Code: schema.ErrAPIKeyMissing, Message: "Missing API key.", Detail: "GROQ_API_KEY"}
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
			// do not retry deterministic provider errors (4xx)
			break
		}

		time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
	}
	return "", lastErr
}

func (c *GroqClient) transcribeOnce(ctx context.Context, audioPath string, opts TranscribeOptions) (string, error) {
	f, err := os.Open(audioPath)
	if err != nil {
		return "", ProviderError{Code: schema.ErrFileUnreadable, Message: "File unreadable.", Detail: err.Error()}
	}
	defer f.Close()

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	contentType := mw.FormDataContentType()

	go func() {
		_ = mw.WriteField("model", c.opts.Model)
		if lang := strings.TrimSpace(opts.Language); lang != "" && strings.ToLower(lang) != "auto" {
			_ = mw.WriteField("language", lang)
		}
		_ = mw.WriteField("response_format", "json")

		part, err := mw.CreateFormFile("file", filepath.Base(audioPath))
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, f); err != nil {
			_ = pw.CloseWithError(err)
			return
		}

		if err := mw.Close(); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		_ = pw.Close()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.opts.BaseURL, "/")+"/openai/v1/audio/transcriptions", pr)
	if err != nil {
		return "", ProviderError{Code: schema.ErrUploadFailed, Message: "Failed to build request.", Detail: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", contentType)

	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", ProviderError{Code: schema.ErrTimeout, Message: "Timed out.", Detail: err.Error()}
		}
		return "", ProviderError{Code: schema.ErrUploadFailed, Message: "Upload failed.", Detail: err.Error()}
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
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

	var tr groqTranscribeResponse
	if err := json.Unmarshal(b, &tr); err != nil {
		return "", ProviderError{Code: schema.ErrProviderError, Message: "Failed to parse provider response.", Detail: err.Error()}
	}
	return tr.Text, nil
}
