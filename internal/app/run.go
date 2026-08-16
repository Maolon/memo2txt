package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"memos2txt/internal/cache"
	"memos2txt/internal/output"
	"memos2txt/internal/providers"
	"memos2txt/internal/schema"
	"memos2txt/internal/secrets"
)

const Version = "0.1.0"

type Config struct {
	File string

	Provider       string
	Model          string
	Language       string
	TimeoutSeconds int
	Diarization    bool
	DiarizationSet bool
	Punctuate      bool

	JSON           bool
	JSONIndent     bool
	InlineMaxChars int
	PreviewLines   int
	Quiet          bool
	Verbose        bool

	NoCache   bool
	MaxFileMB int
	Store     string // "keychain" | "file"

	AutoPreprocess         bool
	PreprocessThresholdMB  int
	ChunkSeconds           int
	PreprocessBitrateKbps  int
	PreprocessSampleRateHz int
	KeepTemp               bool

	Setup       bool
	UnsetAPIKey bool
	APIKey      string
	APIKeyStdin bool

	ParseError string
}

func Run(ctx context.Context, cfg Config) (schema.Response, error) {
	if cfg.Setup {
		return runSetup(cfg)
	}
	return runTranscribe(ctx, cfg)
}

func runSetup(cfg Config) (schema.Response, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		return schema.ErrorResponse(schema.ErrInvalidArgs, "Missing --provider in setup mode.", ""), nil
	}

	envVar, ok := providers.EnvVarForProvider(provider)
	if !ok {
		return schema.ErrorResponse(schema.ErrInvalidArgs, "Unsupported provider.", "provider="+provider), nil
	}

	store, err := secrets.NewStore(resolveStore(cfg.Store), "memos2txt")
	if err != nil {
		return schema.ErrorResponse(schema.ErrProviderError, "Failed to initialize secret store.", err.Error()), nil
	}

	if cfg.UnsetAPIKey {
		if err := store.Delete(envVar); err != nil {
			return schema.ErrorResponse(schema.ErrProviderError, "Failed to delete API key.", err.Error()), nil
		}
		return schema.Response{
			OK:       true,
			Mode:     "setup",
			Provider: provider,
			Setup: &schema.SetupInfo{
				Store:  store.Kind(),
				EnvVar: envVar,
			},
			Note: "API key deleted.",
		}, nil
	}

	apiKey, err := secrets.ResolveAPIKey(cfg.APIKey, cfg.APIKeyStdin, envVar)
	if err != nil {
		return schema.ErrorResponse(schema.ErrInvalidArgs, "Failed to read API key.", err.Error()), nil
	}
	if apiKey == "" {
		return schema.ErrorResponse(schema.ErrInvalidArgs, "Missing API key.", "Use --api-key, --api-key-stdin, or interactive input."), nil
	}

	if err := store.Set(envVar, apiKey); err != nil {
		return schema.ErrorResponse(schema.ErrProviderError, "Failed to store API key.", err.Error()), nil
	}

	return schema.Response{
		OK:       true,
		Mode:     "setup",
		Provider: provider,
		Setup: &schema.SetupInfo{
			Store:  store.Kind(),
			EnvVar: envVar,
		},
		Note: "API key stored. Runtime lookup order: env var, then configured store.",
	}, nil
}

func runTranscribe(ctx context.Context, cfg Config) (schema.Response, error) {
	if !cfg.JSON {
		return schema.ErrorResponse(schema.ErrInvalidArgs, "--json must be true (stdout contract is JSON).", ""), nil
	}

	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		return schema.ErrorResponse(schema.ErrInvalidArgs, "Missing --provider.", ""), nil
	}

	audioPath, inputInfo, errResp := resolveInput(cfg)
	if errResp != nil {
		return *errResp, nil
	}

	if cfg.MaxFileMB > 0 && inputInfo.FileSizeBytes > int64(cfg.MaxFileMB)*1024*1024 {
		return schema.ErrorResponse(schema.ErrInvalidArgs, "File too large.", fmt.Sprintf("file_size_bytes=%d max_file_mb=%d", inputInfo.FileSizeBytes, cfg.MaxFileMB)), nil
	}

	opts := providers.TranscribeOptions{
		Language:    cfg.Language,
		Diarization: cfg.Diarization,
		Punctuate:   cfg.Punctuate,
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = providers.DefaultModel(provider)
	}

	// Deepgram diarization support: default-enable unless explicitly set by user.
	if provider == "deepgram" && !cfg.DiarizationSet {
		opts.Diarization = true
	}

	cacheDir, err := defaultCacheDir()
	if err != nil {
		return schema.ErrorResponse(schema.ErrProviderError, "Failed to resolve cache dir.", err.Error()), nil
	}
	transcriptStore := cache.NewTranscriptStore(filepath.Join(cacheDir, "transcripts"))

	hash, err := transcriptStore.HashFile(audioPath)
	if err != nil {
		return schema.ErrorResponse(schema.ErrFileUnreadable, "Failed to read audio file.", err.Error()), nil
	}

	key := cache.Key{
		SHA256:      hash,
		Provider:    provider,
		Model:       model,
		Language:    opts.Language,
		Diarization: opts.Diarization,
		Punctuate:   opts.Punctuate,
	}

	transcriptPath := transcriptStore.TranscriptPath(key)
	metaPath := transcriptStore.MetadataPath(key)

	if !cfg.NoCache {
		if text, ok, err := transcriptStore.ReadIfExists(transcriptPath); err != nil {
			return schema.ErrorResponse(schema.ErrProviderError, "Failed to read cached transcript.", err.Error()), nil
		} else if ok {
			out, note := output.Build(text, transcriptPath, cfg.InlineMaxChars, cfg.PreviewLines)
			return schema.Response{
				OK:       true,
				CacheHit: true,
				Mode:     "transcribe",
				Provider: provider,
				Model:    model,
				Options: &schema.Options{
					Language:    opts.Language,
					Diarization: opts.Diarization,
					Punctuate:   opts.Punctuate,
				},
				Input:   &inputInfo,
				Output:  &out,
				Note:    note,
				Version: Version,
			}, nil
		}
	}

	envVar, ok := providers.EnvVarForProvider(provider)
	if !ok {
		return schema.ErrorResponse(schema.ErrInvalidArgs, "Unsupported provider.", "provider="+provider), nil
	}

	apiKey := strings.TrimSpace(os.Getenv(envVar))
	if apiKey == "" {
		store, err := secrets.NewStore(resolveStore(cfg.Store), "memos2txt")
		if err == nil {
			if v, getErr := store.Get(envVar); getErr == nil {
				apiKey = v
			}
		}
	}
	if apiKey == "" {
		return schema.ErrorResponse(schema.ErrAPIKeyMissing, "Missing API key.", "Expected env var "+envVar+" or configured store."), nil
	}

	var transcriptText string
	var ppInfo *schema.PreprocessInfo
	switch provider {
	case "groq":
		client := providers.NewGroqClient(apiKey, providers.GroqClientOptions{
			Model:         model,
			Timeout:       time.Duration(cfg.TimeoutSeconds) * time.Second,
			BaseURL:       os.Getenv("MEMOS2TXT_GROQ_BASE_URL"),
			VerboseStderr: cfg.Verbose && !cfg.Quiet,
		})
		transcriber := providers.FileTranscriber(client)

		shouldPreprocess := cfg.AutoPreprocess && cfg.PreprocessThresholdMB > 0 &&
			inputInfo.FileSizeBytes >= int64(cfg.PreprocessThresholdMB)*1024*1024

		if shouldPreprocess {
			var err error
			transcriptText, ppInfo, err = transcribeWithPreprocess(ctx, transcriber, key, audioPath, opts, cfg, "threshold")
			if err != nil {
				code, msg, detail := providers.MapProviderError(err)
				return schema.ErrorResponse(code, msg, detail), nil
			}
			break
		}

		var err error
		transcriptText, err = transcriber.Transcribe(ctx, audioPath, opts)
		if err != nil {
			// Retry once using preprocess if provider rejects size.
			if cfg.AutoPreprocess && providers.IsRequestTooLarge(err) {
				transcriptText, ppInfo, err = transcribeWithPreprocess(ctx, transcriber, key, audioPath, opts, cfg, "provider_413")
			}
		}
		if err != nil {
			code, msg, detail := providers.MapProviderError(err)
			return schema.ErrorResponse(code, msg, detail), nil
		}
	case "deepgram":
		client := providers.NewDeepgramClient(apiKey, providers.DeepgramClientOptions{
			Model:         model,
			Timeout:       time.Duration(cfg.TimeoutSeconds) * time.Second,
			BaseURL:       os.Getenv("MEMOS2TXT_DEEPGRAM_BASE_URL"),
			VerboseStderr: cfg.Verbose && !cfg.Quiet,
		})
		transcriber := providers.FileTranscriber(client)

		shouldPreprocess := cfg.AutoPreprocess && cfg.PreprocessThresholdMB > 0 &&
			inputInfo.FileSizeBytes >= int64(cfg.PreprocessThresholdMB)*1024*1024

		if shouldPreprocess {
			var err error
			transcriptText, ppInfo, err = transcribeWithPreprocess(ctx, transcriber, key, audioPath, opts, cfg, "threshold")
			if err != nil {
				code, msg, detail := providers.MapProviderError(err)
				return schema.ErrorResponse(code, msg, detail), nil
			}
			break
		}

		var err error
		transcriptText, err = transcriber.Transcribe(ctx, audioPath, opts)
		if err != nil {
			if cfg.AutoPreprocess && providers.IsRequestTooLarge(err) {
				transcriptText, ppInfo, err = transcribeWithPreprocess(ctx, transcriber, key, audioPath, opts, cfg, "provider_413")
			}
		}
		if err != nil {
			code, msg, detail := providers.MapProviderError(err)
			return schema.ErrorResponse(code, msg, detail), nil
		}
	case "assemblyai":
		client := providers.NewAssemblyAIClient(apiKey, providers.AssemblyAIClientOptions{
			Model:         model,
			Timeout:       time.Duration(cfg.TimeoutSeconds) * time.Second,
			BaseURL:       os.Getenv("MEMOS2TXT_ASSEMBLYAI_BASE_URL"),
			VerboseStderr: cfg.Verbose && !cfg.Quiet,
		})
		transcriber := providers.FileTranscriber(client)

		shouldPreprocess := cfg.AutoPreprocess && cfg.PreprocessThresholdMB > 0 &&
			inputInfo.FileSizeBytes >= int64(cfg.PreprocessThresholdMB)*1024*1024

		if shouldPreprocess {
			var err error
			transcriptText, ppInfo, err = transcribeWithPreprocess(ctx, transcriber, key, audioPath, opts, cfg, "threshold")
			if err != nil {
				code, msg, detail := providers.MapProviderError(err)
				return schema.ErrorResponse(code, msg, detail), nil
			}
			break
		}

		var err error
		transcriptText, err = transcriber.Transcribe(ctx, audioPath, opts)
		if err != nil {
			if cfg.AutoPreprocess && providers.IsRequestTooLarge(err) {
				transcriptText, ppInfo, err = transcribeWithPreprocess(ctx, transcriber, key, audioPath, opts, cfg, "provider_413")
			}
		}
		if err != nil {
			code, msg, detail := providers.MapProviderError(err)
			return schema.ErrorResponse(code, msg, detail), nil
		}
	default:
		return schema.ErrorResponse(schema.ErrInvalidArgs, "Unsupported provider.", "provider="+provider), nil
	}

	if err := transcriptStore.WriteTranscript(transcriptPath, transcriptText); err != nil {
		return schema.ErrorResponse(schema.ErrTranscriptWriteFailed, "Failed to write transcript.", err.Error()), nil
	}

	_ = transcriptStore.WriteMetadata(metaPath, cache.Metadata{
		Version:        1,
		Key:            key,
		InputFilePath:  audioPath,
		InputSizeBytes: inputInfo.FileSizeBytes,
		InputMtimeUnix: inputInfo.MtimeUnix,
		CreatedUnix:    time.Now().Unix(),
	})

	out, note := output.Build(transcriptText, transcriptPath, cfg.InlineMaxChars, cfg.PreviewLines)
	return schema.Response{
		OK:       true,
		CacheHit: false,
		Mode:     "transcribe",
		Provider: provider,
		Model:    model,
		Options: &schema.Options{
			Language:    opts.Language,
			Diarization: opts.Diarization,
			Punctuate:   opts.Punctuate,
		},
		Input:      &inputInfo,
		Output:     &out,
		Preprocess: ppInfo,
		Note:       note,
		Version:    Version,
	}, nil
}

func resolveInput(cfg Config) (string, schema.InputInfo, *schema.Response) {
	if strings.TrimSpace(cfg.File) != "" {
		p := cfg.File
		st, err := os.Stat(p)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				r := schema.ErrorResponse(schema.ErrFileNotFound, "File not found.", p)
				return "", schema.InputInfo{}, &r
			}
			r := schema.ErrorResponse(schema.ErrFileUnreadable, "File unreadable.", err.Error())
			return "", schema.InputInfo{}, &r
		}
		if st.IsDir() {
			r := schema.ErrorResponse(schema.ErrInvalidArgs, "--file must point to a file.", p)
			return "", schema.InputInfo{}, &r
		}
		return p, schema.InputInfo{
			FilePath:      p,
			FileSizeBytes: st.Size(),
			MtimeUnix:     st.ModTime().Unix(),
		}, nil
	}

	r := schema.ErrorResponse(schema.ErrInvalidArgs, "Missing --file.", "")
	return "", schema.InputInfo{}, &r
}

func defaultCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".memos2txt"), nil
}

func resolveStore(store string) string {
	s := strings.ToLower(strings.TrimSpace(store))
	if s != "" {
		return s
	}
	if runtime.GOOS == "darwin" {
		return "keychain"
	}
	return "file"
}

func transcribeWithPreprocess(
	ctx context.Context,
	transcriber providers.FileTranscriber,
	key cache.Key,
	originalAudioPath string,
	opts providers.TranscribeOptions,
	cfg Config,
	reason string,
) (string, *schema.PreprocessInfo, error) {
	cacheDir, err := defaultCacheDir()
	if err != nil {
		return "", nil, providers.ProviderError{Code: schema.ErrProviderError, Message: "Failed to resolve cache dir.", Detail: err.Error()}
	}
	tmpBase := filepath.Join(cacheDir, "tmp", key.SHA256)
	normalized := filepath.Join(tmpBase, "normalized.mp3")
	chunksDir := filepath.Join(tmpBase, "chunks")

	// Resolve effective chunk size: explicit >0 forces chunking; 0=auto, probe
	// duration and chunk only when input exceeds Deepgram sync /v1/listen limit.
	// ponytail: hardcoded 3600s threshold matches Deepgram's documented limit;
	// probe errors silently fall back to single-shot (preserves prior behavior).
	effectiveChunk := cfg.ChunkSeconds
	if effectiveChunk == 0 {
		if dur, perr := providers.ProbeDurationSeconds(ctx, originalAudioPath); perr == nil && dur > 3600 {
			effectiveChunk = 600
		}
	}

	ppOpts := providers.PreprocessOptions{
		ChunkSeconds:  effectiveChunk,
		BitrateKbps:   cfg.PreprocessBitrateKbps,
		SampleRateHz:  cfg.PreprocessSampleRateHz,
		VerboseStderr: cfg.Verbose && !cfg.Quiet,
	}

	startAll := time.Now()
	pp := &schema.PreprocessInfo{
		Triggered:    true,
		Reason:       reason,
		ThresholdMB:  cfg.PreprocessThresholdMB,
		TempKept:     cfg.KeepTemp,
		ChunkSeconds: effectiveChunk,
	}

	t0 := time.Now()
	if err := providers.NormalizeToMP3(ctx, originalAudioPath, normalized, ppOpts); err != nil {
		return "", nil, err
	}
	pp.NormalizeSeconds = time.Since(t0).Seconds()
	if st, err := os.Stat(normalized); err == nil {
		pp.NormalizedSizeBytes = st.Size()
		if cfg.KeepTemp {
			pp.NormalizedPath = normalized
		}
	}

	// Skip single-shot when chunking is enabled. Deepgram sync /v1/listen
	// silently returns HTTP 200 + empty channels for >1h audio (not a 413),
	// so probing the API on long input is wasted time and a confusing error.
	if effectiveChunk <= 0 {
		t1 := time.Now()
		text, err := transcriber.Transcribe(ctx, normalized, opts)
		pp.NormalizedTranscribeSeconds = time.Since(t1).Seconds()
		if err == nil {
			pp.Mode = "normalized"
			pp.TotalSeconds = time.Since(startAll).Seconds()
			if !cfg.KeepTemp {
				_ = os.RemoveAll(tmpBase)
			}
			return text, pp, nil
		}
		return "", pp, err
	}

	t2 := time.Now()
	chunks, err := providers.SegmentToChunks(ctx, normalized, chunksDir, ppOpts)
	if err != nil {
		return "", pp, err
	}
	if len(chunks) == 0 {
		return "", pp, providers.ProviderError{Code: schema.ErrProviderError, Message: "Preprocess produced no chunks.", Detail: ""}
	}
	pp.SegmentSeconds = time.Since(t2).Seconds()
	pp.Mode = "chunked"
	pp.ChunkCount = len(chunks)
	if cfg.KeepTemp {
		pp.ChunkPaths = append([]string{}, chunks...)
	}

	var b strings.Builder
	pp.ChunkTranscribeSeconds = make([]float64, 0, len(chunks))
	for i, chunk := range chunks {
		if ctx.Err() != nil {
			return "", pp, providers.ProviderError{Code: schema.ErrTimeout, Message: "Timed out.", Detail: ctx.Err().Error()}
		}
		tc := time.Now()
		t, err := transcriber.Transcribe(ctx, chunk, opts)
		pp.ChunkTranscribeSeconds = append(pp.ChunkTranscribeSeconds, time.Since(tc).Seconds())
		if err != nil {
			return "", pp, err
		}
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.TrimSpace(t))
		b.WriteString("\n")
	}

	if !cfg.KeepTemp {
		_ = os.RemoveAll(tmpBase)
	}

	pp.TotalSeconds = time.Since(startAll).Seconds()
	return b.String(), pp, nil
}
