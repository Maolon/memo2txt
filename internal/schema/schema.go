package schema

type ErrorCode string

const (
	ErrNoMatch               ErrorCode = "NO_MATCH"
	ErrFileNotFound          ErrorCode = "FILE_NOT_FOUND"
	ErrFileUnreadable        ErrorCode = "FILE_UNREADABLE"
	ErrAPIKeyMissing         ErrorCode = "API_KEY_MISSING"
	ErrUploadFailed          ErrorCode = "UPLOAD_FAILED"
	ErrTimeout               ErrorCode = "TIMEOUT"
	ErrProviderError         ErrorCode = "PROVIDER_ERROR"
	ErrTranscriptWriteFailed ErrorCode = "TRANSCRIPT_WRITE_FAILED"
	ErrInvalidArgs           ErrorCode = "INVALID_ARGS"
)

type Response struct {
	OK         bool            `json:"ok"`
	CacheHit   bool            `json:"cache_hit,omitempty"`
	Provider   string          `json:"provider,omitempty"`
	Model      string          `json:"model,omitempty"`
	Mode       string          `json:"mode,omitempty"`
	Options    *Options        `json:"options,omitempty"`
	Input      *InputInfo      `json:"input,omitempty"`
	Output     *OutputInfo     `json:"output,omitempty"`
	Preprocess *PreprocessInfo `json:"preprocess,omitempty"`
	Note       string          `json:"note,omitempty"`
	Error      *ErrorInfo      `json:"error,omitempty"`
	Setup      *SetupInfo      `json:"setup,omitempty"`
	Version    string          `json:"version,omitempty"`
}

type Options struct {
	Language    string `json:"language,omitempty"`
	Diarization bool   `json:"diarization,omitempty"`
	Punctuate   bool   `json:"punctuate,omitempty"`
}

type InputInfo struct {
	FilePath      string `json:"file_path,omitempty"`
	FileSizeBytes int64  `json:"file_size_bytes,omitempty"`
	MtimeUnix     int64  `json:"mtime_unix,omitempty"`
}

type OutputInfo struct {
	TranscriptPath    string   `json:"transcript_path,omitempty"`
	InlineMode        string   `json:"inline_mode,omitempty"` // "full" | "preview"
	TranscriptFull    string   `json:"transcript_full,omitempty"`
	TranscriptPreview []string `json:"transcript_preview,omitempty"`
	Chars             int      `json:"chars,omitempty"`
	Lines             int      `json:"lines,omitempty"`
}

type ErrorInfo struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Detail  string    `json:"detail,omitempty"`
}

type SetupInfo struct {
	Store  string `json:"store,omitempty"`
	EnvVar string `json:"env_var,omitempty"`
}

type PreprocessInfo struct {
	Triggered                   bool      `json:"triggered"`
	Reason                      string    `json:"reason,omitempty"` // "threshold" | "provider_413"
	Mode                        string    `json:"mode,omitempty"`   // "normalized" | "chunked"
	ThresholdMB                 int       `json:"threshold_mb,omitempty"`
	TempKept                    bool      `json:"temp_kept,omitempty"`
	NormalizedPath              string    `json:"normalized_path,omitempty"`
	NormalizedSizeBytes         int64     `json:"normalized_size_bytes,omitempty"`
	NormalizeSeconds            float64   `json:"normalize_seconds,omitempty"`
	NormalizedTranscribeSeconds float64   `json:"normalized_transcribe_seconds,omitempty"`
	ChunkSeconds                int       `json:"chunk_seconds,omitempty"`
	SegmentSeconds              float64   `json:"segment_seconds,omitempty"`
	ChunkCount                  int       `json:"chunk_count,omitempty"`
	ChunkPaths                  []string  `json:"chunk_paths,omitempty"`
	ChunkTranscribeSeconds      []float64 `json:"chunk_transcribe_seconds,omitempty"`
	TotalSeconds                float64   `json:"total_seconds,omitempty"`
}

func ErrorResponse(code ErrorCode, message, detail string) Response {
	return Response{
		OK: false,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
			Detail:  detail,
		},
	}
}
