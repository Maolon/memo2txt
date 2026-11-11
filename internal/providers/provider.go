package providers

import (
	"context"
	"errors"

	"memos2txt/internal/schema"
)

// FileTranscriber transcribes a local audio file to plain text.
type FileTranscriber interface {
	Transcribe(ctx context.Context, audioPath string, opts TranscribeOptions) (string, error)
}

// SegmentTranscriber exposes timed segments so local diarization can assign
// speakers to each utterance. Implemented by local ASR providers.
type SegmentTranscriber interface {
	TranscribeSegments(ctx context.Context, audioPath string, opts TranscribeOptions) ([]Segment, error)
}

type TranscribeOptions struct {
	Language      string
	Diarization   bool
	Punctuate     bool
	Timestamps    bool
	InitialPrompt string
}

// Segment is a timed ASR fragment. Words is optional and populated by engines
// that can emit word-level timestamps.
type Segment struct {
	Start float64
	End   float64
	Text  string
	Words []Word
}

type Word struct {
	Text  string
	Start float64
	End   float64
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
