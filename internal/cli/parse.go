package cli

import (
	"bytes"
	"flag"
	"strconv"
	"strings"

	"memos2txt/internal/app"
	"memos2txt/internal/providers"
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
	if len(args) > 0 && args[0] == "diarizer" {
		return parseDiarizer(args[1:])
	}

	var cfg app.Config

	fs := flag.NewFlagSet("memos2txt", flag.ContinueOnError)
	fs.SetOutput(&bytes.Buffer{})

	var help bool
	fs.BoolVar(&help, "h", false, "Show help and exit.")
	fs.BoolVar(&help, "help", false, "Show help and exit.")

	fs.StringVar(&cfg.File, "file", "", "Direct audio file path to transcribe (required).")

	fs.StringVar(&cfg.Provider, "provider", "", "Transcription provider. Cloud: groq|deepgram|assemblyai. Local: mlx-whisper.")
	fs.StringVar(&cfg.Model, "model", "", "Provider model (optional). Defaults: groq=whisper-large-v3, deepgram=nova-3, assemblyai=best, mlx-whisper=mlx-community/whisper-large-v3-turbo.")
	fs.StringVar(&cfg.Language, "language", "auto", "Language hint (auto|en|zh|...). 'auto' omits the hint.")
	fs.Var(&intWithSetFlag{Value: &cfg.TimeoutSeconds, WasSet: &cfg.TimeoutSet}, "timeout", "Timeout in seconds (default 300; 3600 for local providers).")
	fs.Var(&boolWithSetFlag{Value: &cfg.Diarization, WasSet: &cfg.DiarizationSet}, "diarization", "Request speaker diarization. Local providers use --diarizer; deepgram defaults to true.")
	fs.BoolVar(&cfg.Punctuate, "punctuate", true, "Request punctuation if supported (default true).")

	fs.StringVar(&cfg.Diarizer, "diarizer", "auto", "Local diarization backend: auto|none|pyannote. auto enables pyannote when --diarization=true.")
	fs.StringVar(&cfg.DiarizerModel, "diarizer-model", "", "pyannote pipeline model (default pyannote/speaker-diarization-community-1).")
	fs.StringVar(&cfg.DiarizerDevice, "diarizer-device", "auto", "Diarization device: auto|cpu|mps|cuda.")
	fs.IntVar(&cfg.NumSpeakers, "num-speakers", 0, "Exact number of speakers (0 = infer).")
	fs.IntVar(&cfg.MinSpeakers, "min-speakers", 0, "Minimum number of speakers (0 = infer).")
	fs.IntVar(&cfg.MaxSpeakers, "max-speakers", 0, "Maximum number of speakers (0 = infer).")
	fs.BoolVar(&cfg.Timestamps, "timestamps", false, "Prefix diarized lines with [MM:SS] timestamps.")
	fs.StringVar(&cfg.WhisperPrompt, "whisper-prompt", "", "Initial prompt for local Whisper (names, jargon).")

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
	if !cfg.TimeoutSet {
		if providers.IsLocal(cfg.Provider) {
			cfg.TimeoutSeconds = 3600
		} else {
			cfg.TimeoutSeconds = 300
		}
	}
	return cfg, ParseResultOK
}

func parseAuth(args []string) (app.Config, ParseResult) {
	var cfg app.Config
	cfg.AuthMode = true
	cfg.JSON = true

	var flags []string
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if (arg == "-store" || arg == "--store" || arg == "-api-key" || arg == "--api-key" ||
				arg == "-provider" || arg == "--provider" || arg == "-adapter" || arg == "--adapter") &&
				i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flags = append(flags, args[i])
			}
		} else {
			positional = append(positional, arg)
		}
	}

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

	if err := fs.Parse(flags); err != nil {
		cfg.ParseError = strings.TrimSpace(err.Error())
		return cfg, ParseResultError
	}
	if help {
		return cfg, ParseResultHelp
	}
	if cfg.Provider == "" && len(positional) > 0 {
		cfg.Provider = strings.TrimSpace(positional[0])
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

type intWithSetFlag struct {
	Value  *int
	WasSet *bool
}

func (i *intWithSetFlag) String() string {
	if i == nil || i.Value == nil {
		return ""
	}
	return strconv.Itoa(*i.Value)
}

func (i *intWithSetFlag) Set(s string) error {
	v, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	if i.Value != nil {
		*i.Value = v
	}
	if i.WasSet != nil {
		*i.WasSet = true
	}
	return nil
}

func parseDiarizer(args []string) (app.Config, ParseResult) {
	var cfg app.Config
	cfg.DiarizerCommand = true
	cfg.JSON = true
	cfg.TimeoutSeconds = 7200

	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if (arg == "--python" || arg == "-python" || arg == "--timeout" || arg == "-timeout") &&
				i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flags = append(flags, args[i])
			}
		} else {
			positional = append(positional, arg)
		}
	}

	fs := flag.NewFlagSet("memos2txt diarizer", flag.ContinueOnError)
	fs.SetOutput(&bytes.Buffer{})

	var help bool
	fs.BoolVar(&help, "h", false, "Show help and exit.")
	fs.BoolVar(&help, "help", false, "Show help and exit.")
	fs.StringVar(&cfg.DiarizerPython, "python", "3.12", "Python version/spec used for the managed environment.")
	fs.BoolVar(&cfg.DiarizerForce, "force", false, "Recreate the environment from scratch.")
	fs.BoolVar(&cfg.JSONIndent, "json-indent", false, "Pretty-print JSON.")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "Verbose install logs.")
	fs.Var(&intWithSetFlag{Value: &cfg.TimeoutSeconds, WasSet: &cfg.TimeoutSet}, "timeout", "Timeout in seconds (default 7200).")

	if err := fs.Parse(flags); err != nil {
		cfg.ParseError = strings.TrimSpace(err.Error())
		return cfg, ParseResultError
	}
	if help {
		return cfg, ParseResultHelp
	}
	if len(positional) > 0 {
		cfg.DiarizerAction = strings.ToLower(strings.TrimSpace(positional[0]))
	} else {
		cfg.DiarizerAction = "status"
	}
	return cfg, ParseResultOK
}
