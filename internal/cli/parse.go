package cli

import (
	"bytes"
	"flag"
	"strconv"
	"strings"

	"memos2txt/internal/app"
)

type ParseResult int

const (
	ParseResultOK ParseResult = iota
	ParseResultHelp
	ParseResultError
)

func Parse(args []string) (app.Config, ParseResult) {
	if len(args) > 0 && args[0] == "auth" {
		return parseAuth(args[1:])
	}

	var cfg app.Config

	fs := flag.NewFlagSet("memos2txt", flag.ContinueOnError)
	fs.SetOutput(&bytes.Buffer{})

	var help bool
	fs.BoolVar(&help, "h", false, "Show help and exit.")
	fs.BoolVar(&help, "help", false, "Show help and exit.")

	fs.StringVar(&cfg.File, "file", "", "Direct audio file path to transcribe (required).")

	fs.StringVar(&cfg.Provider, "provider", "", "Transcription provider: groq|deepgram|assemblyai.")
	fs.StringVar(&cfg.Model, "model", "", "Provider model (optional). Defaults: groq=whisper-large-v3, deepgram=nova-3, assemblyai=best.")
	fs.StringVar(&cfg.Language, "language", "auto", "Language hint (auto|en|zh|...). 'auto' omits the hint.")
	fs.IntVar(&cfg.TimeoutSeconds, "timeout", 300, "Timeout in seconds (default 300).")
	fs.Var(&boolWithSetFlag{Value: &cfg.Diarization, WasSet: &cfg.DiarizationSet}, "diarization", "Request diarization if supported. For deepgram defaults to true unless explicitly set.")
	fs.BoolVar(&cfg.Punctuate, "punctuate", true, "Request punctuation if supported (default true).")

	fs.BoolVar(&cfg.JSON, "json", true, "Output JSON to stdout (default true).")
	fs.BoolVar(&cfg.JSONIndent, "json-indent", false, "Pretty-print JSON (default false).")
	fs.IntVar(&cfg.InlineMaxChars, "inline-max-chars", 4000, "Inline full transcript if <= this many chars (default 4000).")
	fs.IntVar(&cfg.PreviewLines, "preview-lines", 5, "Preview lines when transcript is too long (default 5).")
	fs.BoolVar(&cfg.Quiet, "quiet", false, "Suppress stderr logs (default false).")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "Verbose stderr logs (default false).")

	fs.BoolVar(&cfg.NoCache, "no-cache", false, "Disable transcript cache (default false).")
	fs.IntVar(&cfg.MaxFileMB, "max-file-mb", 200, "Reject files larger than this many MB (default 200).")
	fs.StringVar(&cfg.Store, "store", "", "API key store for runtime lookup: keychain|file (default: keychain on macOS, file elsewhere).")

	fs.BoolVar(&cfg.AutoPreprocess, "auto-preprocess", true, "Auto preprocess large files (ffmpeg re-encode + chunk) to avoid provider size limits (default true).")
	fs.IntVar(&cfg.PreprocessThresholdMB, "preprocess-threshold-mb", 25, "Preprocess when input file size >= this many MB (default 25).")
	fs.IntVar(&cfg.ChunkSeconds, "chunk-seconds", 0, "Chunk size in seconds. 0=auto (probe duration, chunk if >1h). >0 forces chunking.")
	fs.IntVar(&cfg.PreprocessBitrateKbps, "preprocess-bitrate-kbps", 32, "Audio bitrate (kbps) when preprocessing (default 32).")
	fs.IntVar(&cfg.PreprocessSampleRateHz, "preprocess-sample-rate-hz", 16000, "Audio sample rate (Hz) when preprocessing (default 16000).")
	fs.BoolVar(&cfg.KeepTemp, "keep-temp", false, "Keep temporary chunk files (default false).")

	var unset, deleteKey bool
	fs.BoolVar(&cfg.Setup, "setup", false, "Setup API key storage and exit (see -h).")
	fs.BoolVar(&cfg.UnsetAPIKey, "unset-api-key", false, "Delete stored API key for --provider and exit (requires --setup).")
	fs.BoolVar(&unset, "unset", false, "Alias for --unset-api-key.")
	fs.BoolVar(&deleteKey, "delete", false, "Alias for --unset-api-key.")
	fs.StringVar(&cfg.APIKey, "api-key", "", "API key value (WARNING: may leak via shell history / process list). Prefer --api-key-stdin or interactive.")
	fs.BoolVar(&cfg.APIKeyStdin, "api-key-stdin", false, "Read API key from stdin (recommended for scripting).")

	if err := fs.Parse(args); err != nil {
		cfg.ParseError = strings.TrimSpace(err.Error())
		return cfg, ParseResultError
	}
	if help {
		return cfg, ParseResultHelp
	}
	if unset || deleteKey {
		cfg.UnsetAPIKey = true
	}
	return cfg, ParseResultOK
}

func parseAuth(args []string) (app.Config, ParseResult) {
	var cfg app.Config
	cfg.AuthMode = true
	cfg.JSON = true

	fs := flag.NewFlagSet("memos2txt auth", flag.ContinueOnError)
	fs.SetOutput(&bytes.Buffer{})

	var help bool
	fs.BoolVar(&help, "h", false, "Show help and exit.")
	fs.BoolVar(&help, "help", false, "Show help and exit.")

	var unset, deleteKey, unsetAPIKey bool
	fs.BoolVar(&unset, "unset", false, "Delete stored API key for adapter.")
	fs.BoolVar(&deleteKey, "delete", false, "Delete stored API key for adapter.")
	fs.BoolVar(&unsetAPIKey, "unset-api-key", false, "Delete stored API key for adapter.")

	fs.StringVar(&cfg.Provider, "provider", "", "Provider/adapter name: groq|deepgram|assemblyai.")
	fs.StringVar(&cfg.Provider, "adapter", "", "Provider/adapter name: groq|deepgram|assemblyai.")
	fs.StringVar(&cfg.APIKey, "api-key", "", "API key value.")
	fs.BoolVar(&cfg.APIKeyStdin, "api-key-stdin", false, "Read API key from stdin.")
	fs.StringVar(&cfg.Store, "store", "", "Storage backend: keychain|file.")
	fs.BoolVar(&cfg.AuthList, "list", false, "List authentication status for all adapters.")
	fs.BoolVar(&cfg.JSONIndent, "json-indent", false, "Pretty-print JSON.")

	var remainingArgs []string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") && cfg.Provider == "" && !cfg.AuthList {
			cfg.Provider = arg
		} else {
			remainingArgs = append(remainingArgs, arg)
		}
	}

	if err := fs.Parse(remainingArgs); err != nil {
		cfg.ParseError = strings.TrimSpace(err.Error())
		return cfg, ParseResultError
	}
	if help {
		return cfg, ParseResultHelp
	}
	cfg.UnsetAPIKey = unset || deleteKey || unsetAPIKey
	return cfg, ParseResultOK
}

type boolWithSetFlag struct {
	Value  *bool
	WasSet *bool
}

func (b *boolWithSetFlag) String() string {
	if b == nil || b.Value == nil {
		return ""
	}
	if *b.Value {
		return "true"
	}
	return "false"
}

func (b *boolWithSetFlag) Set(s string) error {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	if b.Value != nil {
		*b.Value = v
	}
	if b.WasSet != nil {
		*b.WasSet = true
	}
	return nil
}

func (b *boolWithSetFlag) IsBoolFlag() bool { return true }
