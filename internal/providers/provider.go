package providers

import (
	"context"
	"errors"

	"memos2txt/internal/schema"
)

type FileTranscriber interface {
	Transcribe(ctx context.Context, audioPath string, opts TranscribeOptions) (string, error)
}

type TranscribeOptions struct {
	Language    string
	Diarization bool
	Punctuate   bool
}

func AllProviders() []string {
	return []string{"groq", "deepgram", "assemblyai"}
}

func EnvVarForProvider(provider string) (string, bool) {
	switch provider {
	case "groq":
		return "GROQ_API_KEY", true
	case "deepgram":
		return "DEEPGRAM_API_KEY", true
	case "assemblyai":
		return "ASSEMBLYAI_API_KEY", true
	default:
		return "", false
	}
}

func DefaultModel(provider string) string {
	switch provider {
	case "groq":
		return "whisper-large-v3"
	case "deepgram":
		return "nova-3"
	case "assemblyai":
		return "best"
	default:
		return ""
	}
}

func MapProviderError(err error) (schema.ErrorCode, string, string) {
	var pe ProviderError
	if errors.As(err, &pe) {
		return pe.Code, pe.Message, pe.Detail
	}
	return schema.ErrProviderError, "Provider error.", err.Error()
}

type ProviderError struct {
	Code    schema.ErrorCode
	Message string
	Detail  string
}

func (e ProviderError) Error() string {
	if e.Detail == "" {
		return e.Message
	}
	return e.Message + ": " + e.Detail
}
