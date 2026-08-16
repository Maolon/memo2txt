package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"memos2txt/internal/schema"
)

type AssemblyAIClientOptions struct {
	Model         string
	Timeout       time.Duration
	BaseURL       string
	VerboseStderr bool
	PollInterval  time.Duration
}

type AssemblyAIClient struct {
	apiKey string
	opts   AssemblyAIClientOptions
	http   *http.Client
}

func NewAssemblyAIClient(apiKey string, opts AssemblyAIClientOptions) *AssemblyAIClient {
	baseTimeout := opts.Timeout
	if baseTimeout <= 0 {
		baseTimeout = 300 * time.Second
	}
	if strings.TrimSpace(opts.BaseURL) == "" {
		opts.BaseURL = "https://api.assemblyai.com"
	}
	if strings.TrimSpace(opts.Model) == "" {
		opts.Model = "best"
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = 2 * time.Second
	}
	return &AssemblyAIClient{
		apiKey: strings.TrimSpace(apiKey),
		opts:   opts,
		http: &http.Client{
			Timeout: baseTimeout,
		},
	}
}

type assemblyUploadResponse struct {
	UploadURL string `json:"upload_url"`
}

type assemblyCreateTranscriptRequest struct {
	AudioURL      string `json:"audio_url"`
	LanguageCode  string `json:"language_code,omitempty"`
	Punctuate     *bool  `json:"punctuate,omitempty"`
	SpeakerLabels *bool  `json:"speaker_labels,omitempty"`
	SpeechModel   string `json:"speech_model,omitempty"`
}

type assemblyCreateTranscriptResponse struct {
	ID string `json:"id"`
}

type assemblyTranscriptResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"` // queued|processing|completed|error
	Text   string `json:"text"`
	Error  string `json:"error"`
}

func (c *AssemblyAIClient) Transcribe(ctx context.Context, audioPath string, opts TranscribeOptions) (string, error) {
	if c.apiKey == "" {
		return "", ProviderError{Code: schema.ErrAPIKeyMissing, Message: "Missing API key.", Detail: "ASSEMBLYAI_API_KEY"}
	}

	uploadURL, err := c.upload(ctx, audioPath)
	if err != nil {
		return "", err
	}

	id, err := c.createTranscript(ctx, uploadURL, opts)
	if err != nil {
		return "", err
	}

	return c.pollTranscript(ctx, id)
}

func (c *AssemblyAIClient) upload(ctx context.Context, audioPath string) (string, error) {
	f, err := os.Open(audioPath)
	if err != nil {
		return "", ProviderError{Code: schema.ErrFileUnreadable, Message: "File unreadable.", Detail: err.Error()}
	}
	defer f.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.opts.BaseURL, "/")+"/v2/upload", f)
	if err != nil {
		return "", ProviderError{Code: schema.ErrUploadFailed, Message: "Failed to build request.", Detail: err.Error()}
	}
	req.Header.Set("Authorization", c.apiKey)
	req.Header.Set("Content-Type", "application/octet-stream")

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

	var ur assemblyUploadResponse
	if err := json.Unmarshal(b, &ur); err != nil {
		return "", ProviderError{Code: schema.ErrProviderError, Message: "Failed to parse provider response.", Detail: err.Error()}
	}
	if strings.TrimSpace(ur.UploadURL) == "" {
		return "", ProviderError{Code: schema.ErrProviderError, Message: "Provider response missing upload_url.", Detail: ""}
	}
	return ur.UploadURL, nil
}

func (c *AssemblyAIClient) createTranscript(ctx context.Context, uploadURL string, opts TranscribeOptions) (string, error) {
	punct := opts.Punctuate
	diar := opts.Diarization

	reqBody := assemblyCreateTranscriptRequest{
		AudioURL:      uploadURL,
		Punctuate:     &punct,
		SpeakerLabels: &diar,
		SpeechModel:   c.opts.Model,
	}
	if lang := strings.TrimSpace(opts.Language); lang != "" && strings.ToLower(lang) != "auto" {
		reqBody.LanguageCode = lang
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", ProviderError{Code: schema.ErrProviderError, Message: "Failed to encode request.", Detail: err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.opts.BaseURL, "/")+"/v2/transcript", bytes.NewReader(b))
	if err != nil {
		return "", ProviderError{Code: schema.ErrUploadFailed, Message: "Failed to build request.", Detail: err.Error()}
	}
	req.Header.Set("Authorization", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", ProviderError{Code: schema.ErrTimeout, Message: "Timed out.", Detail: err.Error()}
		}
		return "", ProviderError{Code: schema.ErrUploadFailed, Message: "Upload failed.", Detail: err.Error()}
	}
	defer resp.Body.Close()

	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code := schema.ErrProviderError
		msg := "Provider error."
		if resp.StatusCode >= 500 {
			code = schema.ErrUploadFailed
			msg = "Provider unavailable."
		}
		return "", ProviderError{
			Code:    code,
			Message: msg,
			Detail:  fmt.Sprintf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(rb))),
		}
	}

	var tr assemblyCreateTranscriptResponse
	if err := json.Unmarshal(rb, &tr); err != nil {
		return "", ProviderError{Code: schema.ErrProviderError, Message: "Failed to parse provider response.", Detail: err.Error()}
	}
	if strings.TrimSpace(tr.ID) == "" {
		return "", ProviderError{Code: schema.ErrProviderError, Message: "Provider response missing transcript id.", Detail: ""}
	}
	return tr.ID, nil
}

func (c *AssemblyAIClient) pollTranscript(ctx context.Context, id string) (string, error) {
	u := strings.TrimRight(c.opts.BaseURL, "/") + "/v2/transcript/" + id

	for {
		if ctx.Err() != nil {
			return "", ProviderError{Code: schema.ErrTimeout, Message: "Timed out.", Detail: ctx.Err().Error()}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return "", ProviderError{Code: schema.ErrProviderError, Message: "Failed to build request.", Detail: err.Error()}
		}
		req.Header.Set("Authorization", c.apiKey)

		resp, err := c.http.Do(req)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return "", ProviderError{Code: schema.ErrTimeout, Message: "Timed out.", Detail: err.Error()}
			}
			return "", ProviderError{Code: schema.ErrUploadFailed, Message: "Upload failed.", Detail: err.Error()}
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		_ = resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			code := schema.ErrProviderError
			msg := "Provider error."
			if resp.StatusCode >= 500 {
				code = schema.ErrUploadFailed
				msg = "Provider unavailable."
			}
			return "", ProviderError{
				Code:    code,
				Message: msg,
				Detail:  fmt.Sprintf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(b))),
			}
		}

		var tr assemblyTranscriptResponse
		if err := json.Unmarshal(b, &tr); err != nil {
			return "", ProviderError{Code: schema.ErrProviderError, Message: "Failed to parse provider response.", Detail: err.Error()}
		}

		switch strings.ToLower(strings.TrimSpace(tr.Status)) {
		case "completed":
			return tr.Text, nil
		case "error":
			return "", ProviderError{Code: schema.ErrProviderError, Message: "Provider error.", Detail: tr.Error}
		default:
			time.Sleep(c.opts.PollInterval)
		}
	}
}
