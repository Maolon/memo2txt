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
	"memos2txt/internal/diarize"
	"memos2txt/internal/output"
	"memos2txt/internal/providers"
	"memos2txt/internal/schema"
	"memos2txt/internal/secrets"
)

const Version = "0.2.0"

type Config struct {
	File string

	Provider       string
	Model          string
	Language       string
	TimeoutSeconds int
	TimeoutSet     bool
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

	// Local provider pipeline (mlx-whisper).
	DiarizerModel  string
	DiarizerDevice string
	NumSpeakers    int
	MinSpeakers    int
	MaxSpeakers    int
	Timestamps     bool
	WhisperPrompt  string

	Setup       bool
	AuthMode    bool
	AuthList    bool
	UnsetAPIKey bool
	APIKey      string
	APIKeyStdin bool

	// `memos2txt diarizer <install|status|uninstall>`.
	DiarizerCommand bool
	DiarizerAction  string
	DiarizerPython  string
	DiarizerForce   bool

	ParseError string
}

func Run(ctx context.Context, cfg Config) (schema.Response, error) {
	if cfg.DiarizerCommand {
		return runDiarizerSetup(ctx, cfg)
	}
	if cfg.Setup || cfg.AuthMode {
		return runSetup(cfg)
	}
	return runTranscribe(ctx, cfg)
}

func runSetup(cfg Config) (schema.Response, error) {
	mode := "setup"
	if cfg.AuthMode {
		mode = "auth"
	}

	provider := providers.NormalizeAuthAdapter(cfg.Provider)
	storeKind := resolveStore(cfg.Store)
	store, err := secrets.NewStore(storeKind, "memos2txt")
	if err != nil {
		return schema.ErrorResponse(schema.ErrProviderError, "Failed to initialize secret store.", err.Error()), nil
	}

	if cfg.AuthList || (cfg.AuthMode && provider == "") {
		status := make(map[string]string)
		for _, p := range providers.AllAuthAdapters() {
			status[p] = credentialStatus(p, store)
		}
		return schema.Response{
			OK:   true,
			Mode: mode,
			Setup: &schema.SetupInfo{
				Store:  store.Kind(),
				Status: status,
			},
			Note:    "Authentication status for available adapters.",
			Version: Version,
		}, nil
	}

	if provider == "" {
		return schema.ErrorResponse(schema.ErrInvalidArgs, "Missing provider/adapter name.", "Example: memos2txt auth groq (cloud ASR key) or memos2txt auth huggingface (HF_TOKEN)."), nil
	}

	envVar, ok := providers.EnvVarForProvider(provider)
	if !ok {
		return schema.ErrorResponse(schema.ErrInvalidArgs, "Unsupported auth adapter.", "adapter="+provider), nil
	}

	if cfg.UnsetAPIKey {
		if err := store.Delete(envVar); err != nil {
			return schema.ErrorResponse(schema.ErrProviderError, "Failed to delete API key.", err.Error()), nil
		}
		return schema.Response{
			OK:       true,
			Mode:     mode,
			Provider: provider,
			Setup: &schema.SetupInfo{
				Store:  store.Kind(),
				EnvVar: envVar,
			},
			Note:    "Credential deleted for " + provider + ".",
			Version: Version,
		}, nil
	}

	apiKey, err := secrets.ResolveAPIKey(cfg.APIKey, cfg.APIKeyStdin, envVar)
	if err != nil {
		return schema.ErrorResponse(schema.ErrInvalidArgs, "Failed to read credential.", err.Error()), nil
	}
	if apiKey == "" {
		return schema.ErrorResponse(schema.ErrInvalidArgs, "Missing credential.", "Use --api-key, --api-key-stdin, or interactive input."), nil
	}

	if err := store.Set(envVar, apiKey); err != nil {
		return schema.ErrorResponse(schema.ErrProviderError, "Failed to store credential.", err.Error()), nil
	}

	return schema.Response{
		OK:       true,
		Mode:     mode,
		Provider: provider,
		Setup: &schema.SetupInfo{
			Store:  store.Kind(),
			EnvVar: envVar,
		},
		Note:    "Credential stored for " + provider + ". Runtime lookup order: env var, then configured store.",
		Version: Version,
	}, nil
}

func runTranscribe(ctx context.Context, cfg Config) (schema.Response, error) {
	if !cfg.JSON {
		return schema.ErrorResponse(schema.ErrInvalidArgs, "--json must be true (stdout contract is JSON).", ""), nil
	}

	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		return schema.ErrorResponse(schema.ErrInvalidArgs, "Missing --provider.", "Cloud: groq|deepgram|assemblyai. Local: mlx-whisper."), nil
	}
	spec, ok := providers.SpecFor(provider)
	if !ok {
		detail := "provider=" + provider
		if strings.HasPrefix(provider, "local") {
			detail = "'local' is a provider category, not a provider. Use --provider mlx-whisper."
		}
		return schema.ErrorResponse(schema.ErrInvalidArgs, "Unsupported provider.", detail), nil
	}

	audioPath, inputInfo, errResp := resolveInput(cfg)
	if errResp != nil {
		return *errResp, nil
	}

	if cfg.MaxFileMB > 0 && inputInfo.FileSizeBytes > int64(cfg.MaxFileMB)*1024*1024 {
		return schema.ErrorResponse(schema.ErrInvalidArgs, "File too large.", fmt.Sprintf("file_size_bytes=%d max_file_mb=%d", inputInfo.FileSizeBytes, cfg.MaxFileMB)), nil
	}

	opts := providers.TranscribeOptions{
		Language:      cfg.Language,
		Diarization:   cfg.Diarization,
		Punctuate:     cfg.Punctuate,
		Timestamps:    cfg.Timestamps,
		InitialPrompt: cfg.WhisperPrompt,
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = providers.DefaultModel(provider)
	}

	cacheDir, err := defaultCacheDir()
	if err != nil {
		return schema.ErrorResponse(schema.ErrProviderError, "Failed to resolve cache dir.", err.Error()), nil
	}

	var diarizer, diarizerModel, localAutoHint string
	if providers.IsLocal(provider) {
		if cfg.ChunkSeconds > 0 {
			return schema.ErrorResponse(schema.ErrInvalidArgs, "Chunking is not supported for local providers.", "mlx-whisper handles long audio internally; remove --chunk-seconds."), nil
		}
		enabled, modelOverride, hint, dErr := resolveLocalDiarization(cfg, cacheDir)
		if dErr != nil {
			return *dErr, nil
		}
		localAutoHint = hint
		if enabled {
			diarizer = "pyannote"
			diarizerModel = modelOverride
			opts.Diarization = true
		} else {
			opts.Diarization = false
		}
	} else {
		// Cloud providers use their native diarization switch. Deepgram
		// defaults to diarization unless the user explicitly disables it.
		opts.Diarization = cfg.Diarization
		if provider == "deepgram" && !cfg.DiarizationSet {
			opts.Diarization = true
		}
	}

	transcriptStore := cache.NewTranscriptStore(filepath.Join(cacheDir, "transcripts"))

	hash, err := transcriptStore.HashFile(audioPath)
	if err != nil {
		return schema.ErrorResponse(schema.ErrFileUnreadable, "Failed to read audio file.", err.Error()), nil
	}

	key := cache.Key{
		SHA256:        hash,
		Provider:      provider,
		Model:         model,
		Language:      opts.Language,
		Diarization:   opts.Diarization,
		Punctuate:     opts.Punctuate,
		Diarizer:      diarizer,
		DiarizerModel: diarizerModel,
	}

	transcriptPath := transcriptStore.TranscriptPath(key)
	metaPath := transcriptStore.MetadataPath(key)

	responseOptions := &schema.Options{
		Language:      opts.Language,
		Diarization:   opts.Diarization,
		Punctuate:     opts.Punctuate,
		Diarizer:      diarizer,
		DiarizerModel: diarizerModel,
		Timestamps:    cfg.Timestamps,
	}

	if !cfg.NoCache {
		if text, ok, err := transcriptStore.ReadIfExists(transcriptPath); err != nil {
			return schema.ErrorResponse(schema.ErrProviderError, "Failed to read cached transcript.", err.Error()), nil
		} else if ok {
			out, note := output.Build(text, transcriptPath, cfg.InlineMaxChars, cfg.PreviewLines)
			return schema.Response{
				OK:          true,
				CacheHit:    true,
				Category:    string(spec.Category),
				Mode:        "transcribe",
				Provider:    provider,
				Model:       model,
				Options:     responseOptions,
				Input:       &inputInfo,
				Output:      &out,
				Diarization: cachedDiarizationInfo(provider, diarizer, diarizerModel, opts.Diarization),
				Note:        joinNotes(note, localAutoHint),
				Version:     Version,
			}, nil
		}
	}

	var transcriptText string
	var ppInfo *schema.PreprocessInfo
	var diarInfo *schema.DiarizationInfo

	if providers.IsLocal(provider) {
		transcriptText, ppInfo, diarInfo, err = transcribeLocal(ctx, cfg, model, audioPath, opts, key, cacheDir, diarizer, diarizerModel)
	} else {
		envVar, _ := providers.EnvVarForProvider(provider)
		apiKey := lookupCredential(envVar, resolveStore(cfg.Store))
		if apiKey == "" {
			return schema.ErrorResponse(schema.ErrAPIKeyMissing, "Missing API key.", "Expected env var "+envVar+" or configured store."), nil
		}
		transcriptText, ppInfo, err = transcribeCloud(ctx, provider, apiKey, model, audioPath, opts, cfg, key)
		if err == nil && opts.Diarization && (provider == "deepgram" || provider == "assemblyai") {
			diarInfo = &schema.DiarizationInfo{
				Enabled:  true,
				Diarizer: provider,
				Model:    model,
			}
		}
	}
	if err != nil {
		code, msg, detail := mapAnyError(err)
		return schema.ErrorResponse(code, msg, detail), nil
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
		OK:          true,
		CacheHit:    false,
		Category:    string(spec.Category),
		Mode:        "transcribe",
		Provider:    provider,
		Model:       model,
		Options:     responseOptions,
		Input:       &inputInfo,
		Output:      &out,
		Preprocess:  ppInfo,
		Diarization: diarInfo,
		Note:        joinNotes(note, localAutoHint),
		Version:     Version,
	}, nil
}

// resolveLocalDiarization decides whether the local provider pipeline includes
// speaker labeling. When --diarization is omitted, local diarization is enabled
// automatically if the managed pyannote runtime and required token are ready.
func resolveLocalDiarization(cfg Config, cacheDir string) (bool, string, string, *schema.Response) {
	if cfg.NumSpeakers < 0 || cfg.MinSpeakers < 0 || cfg.MaxSpeakers < 0 {
		r := schema.ErrorResponse(schema.ErrInvalidArgs, "Speaker counts must be positive.", "")
		return false, "", "", &r
	}
	if cfg.NumSpeakers > 0 && (cfg.MinSpeakers > 0 || cfg.MaxSpeakers > 0) {
		r := schema.ErrorResponse(schema.ErrInvalidArgs, "Conflicting speaker counts.", "Use either --num-speakers or --min-speakers/--max-speakers, not both.")
		return false, "", "", &r
	}
	if cfg.MinSpeakers > 0 && cfg.MaxSpeakers > 0 && cfg.MinSpeakers > cfg.MaxSpeakers {
		r := schema.ErrorResponse(schema.ErrInvalidArgs, "Invalid speaker range.", "--min-speakers must be <= --max-speakers.")
		return false, "", "", &r
	}
	switch strings.ToLower(strings.TrimSpace(cfg.DiarizerDevice)) {
	case "", "auto", "cpu", "mps", "cuda":
	default:
		r := schema.ErrorResponse(schema.ErrInvalidArgs, "Unsupported diarization device.", "device="+cfg.DiarizerDevice+" (supported: auto, cpu, mps, cuda)")
		return false, "", "", &r
	}

	model := strings.TrimSpace(cfg.DiarizerModel)
	if model == "" {
		model = diarize.DefaultModel
	}
	needToken := !isLocalModelPath(model)
	installed := diarize.IsReady(cacheDir)
	tokenReady := !needToken || resolveHFToken(cfg) != ""

	enabled := cfg.Diarization
	hint := ""
	if !cfg.DiarizationSet {
		enabled = installed && tokenReady
		if !enabled {
			switch {
			case !installed:
				hint = "Local diarization auto-disabled: pyannote runtime is not installed. Run `memos2txt diarizer install` to enable speaker labels."
			case !tokenReady:
				hint = "Local diarization auto-disabled: HF_TOKEN is not configured. Run `echo \"$HF_TOKEN\" | memos2txt auth huggingface --api-key-stdin` to enable speaker labels."
			}
		}
	}
	if !enabled {
		return false, "", hint, nil
	}
	return true, model, hint, nil
}

func transcribeCloud(
	ctx context.Context,
	provider string,
	apiKey string,
	model string,
	audioPath string,
	opts providers.TranscribeOptions,
	cfg Config,
	key cache.Key,
) (string, *schema.PreprocessInfo, error) {
	var transcriber providers.FileTranscriber
	switch provider {
	case "groq":
		transcriber = providers.FileTranscriber(providers.NewGroqClient(apiKey, providers.GroqClientOptions{
			Model:         model,
			Timeout:       time.Duration(cfg.TimeoutSeconds) * time.Second,
			BaseURL:       os.Getenv("MEMOS2TXT_GROQ_BASE_URL"),
			VerboseStderr: cfg.Verbose && !cfg.Quiet,
		}))
	case "deepgram":
		transcriber = providers.FileTranscriber(providers.NewDeepgramClient(apiKey, providers.DeepgramClientOptions{
			Model:         model,
			Timeout:       time.Duration(cfg.TimeoutSeconds) * time.Second,
			BaseURL:       os.Getenv("MEMOS2TXT_DEEPGRAM_BASE_URL"),
			VerboseStderr: cfg.Verbose && !cfg.Quiet,
		}))
	case "assemblyai":
		transcriber = providers.FileTranscriber(providers.NewAssemblyAIClient(apiKey, providers.AssemblyAIClientOptions{
			Model:         model,
			Timeout:       time.Duration(cfg.TimeoutSeconds) * time.Second,
			BaseURL:       os.Getenv("MEMOS2TXT_ASSEMBLYAI_BASE_URL"),
			VerboseStderr: cfg.Verbose && !cfg.Quiet,
		}))
	default:
		return "", nil, providers.ProviderError{Code: schema.ErrInvalidArgs, Message: "Unsupported provider.", Detail: provider}
	}

	shouldPreprocess := cfg.AutoPreprocess && cfg.PreprocessThresholdMB > 0 &&
		fileSizeBytes(audioPath) >= int64(cfg.PreprocessThresholdMB)*1024*1024

	if shouldPreprocess {
		return transcribeWithPreprocess(ctx, transcriber, key, audioPath, opts, cfg, "threshold")
	}

	text, err := transcriber.Transcribe(ctx, audioPath, opts)
	if err != nil && cfg.AutoPreprocess && providers.IsRequestTooLarge(err) {
		return transcribeWithPreprocess(ctx, transcriber, key, audioPath, opts, cfg, "provider_413")
	}
	return text, nil, err
}

// transcribeLocal is the local provider pipeline. Like Deepgram's cloud
// provider, it performs ASR and (when enabled) speaker diarization as one
// operation and returns a single transcript artifact.
func transcribeLocal(
	ctx context.Context,
	cfg Config,
	model string,
	audioPath string,
	opts providers.TranscribeOptions,
	key cache.Key,
	cacheDir string,
	diarizer string,
	diarizerModel string,
) (string, *schema.PreprocessInfo, *schema.DiarizationInfo, error) {
	if _, err := providers.DetectMLXWhisperBinary(); err != nil {
		return "", nil, nil, err
	}
	client := providers.NewMLXWhisperClient(providers.MLXWhisperClientOptions{
		Model:         model,
		VerboseStderr: cfg.Verbose && !cfg.Quiet,
	})

	ppOpts := providers.PreprocessOptions{
		SampleRateHz:  cfg.PreprocessSampleRateHz,
		VerboseStderr: cfg.Verbose && !cfg.Quiet,
	}
	if ppOpts.SampleRateHz <= 0 {
		ppOpts.SampleRateHz = 16000
	}
	if diarizer != "" {
		// pyannote community-1 is trained around 16kHz audio; keep the local
		// diarization WAV independent of cloud preprocessing overrides.
		ppOpts.SampleRateHz = 16000
	}

	if diarizer == "" {
		if isDecodableAudio(audioPath) {
			text, err := client.Transcribe(ctx, audioPath, opts)
			return text, nil, nil, err
		}
		// mlx_whisper can decode most containers through ffmpeg, but normalize
		// unknown extensions to a predictable input.
		tmpDir := filepath.Join(cacheDir, "tmp", key.SHA256+"-local")
		wavPath := filepath.Join(tmpDir, "audio.wav")
		defer cleanupTemp(tmpDir, cfg.KeepTemp)
		start := time.Now()
		if err := providers.NormalizeToWAV(ctx, audioPath, wavPath, ppOpts); err != nil {
			return "", nil, nil, err
		}
		pp := &schema.PreprocessInfo{
			Triggered:      true,
			Reason:         "local_normalize",
			Mode:           "normalized",
			TempKept:       cfg.KeepTemp,
			NormalizedPath: conditionalPath(wavPath, cfg.KeepTemp),
		}
		pp.NormalizeSeconds = time.Since(start).Seconds()
		if st, err := os.Stat(wavPath); err == nil {
			pp.NormalizedSizeBytes = st.Size()
		}
		text, err := client.Transcribe(ctx, wavPath, opts)
		return text, pp, nil, err
	}

	hfToken := resolveHFToken(cfg)
	if hfToken == "" && !isLocalModelPath(diarizerModel) {
		return "", nil, nil, providers.ProviderError{
			Code:    schema.ErrAPIKeyMissing,
			Message: "Missing Hugging Face token for the diarization model.",
			Detail: "Accept the terms at https://huggingface.co/" + diarizerModel +
				" and run: echo \"$HF_TOKEN\" | memos2txt auth huggingface --api-key-stdin",
		}
	}

	tmpDir := filepath.Join(cacheDir, "tmp", key.SHA256+"-local")
	wavPath := filepath.Join(tmpDir, "audio.wav")
	defer cleanupTemp(tmpDir, cfg.KeepTemp)

	startAll := time.Now()
	pp := &schema.PreprocessInfo{
		Triggered:      true,
		Reason:         "local_diarization",
		Mode:           "normalized",
		TempKept:       cfg.KeepTemp,
		NormalizedPath: conditionalPath(wavPath, cfg.KeepTemp),
	}
	normStart := time.Now()
	if err := providers.NormalizeToWAV(ctx, audioPath, wavPath, ppOpts); err != nil {
		return "", pp, nil, err
	}
	pp.NormalizeSeconds = time.Since(normStart).Seconds()
	if st, err := os.Stat(wavPath); err == nil {
		pp.NormalizedSizeBytes = st.Size()
	}

	dres, err := diarize.Run(ctx, wavPath, diarize.Options{
		Model:       diarizerModel,
		Device:      cfg.DiarizerDevice,
		HFTOKEN:     hfToken,
		NumSpeakers: cfg.NumSpeakers,
		MinSpeakers: cfg.MinSpeakers,
		MaxSpeakers: cfg.MaxSpeakers,
		Verbose:     cfg.Verbose && !cfg.Quiet,
		CacheDir:    cacheDir,
	})
	if err != nil {
		return "", pp, nil, err
	}

	asrStart := time.Now()
	segments, err := client.TranscribeSegments(ctx, wavPath, opts)
	if err != nil {
		return "", pp, nil, err
	}
	pp.NormalizedTranscribeSeconds = time.Since(asrStart).Seconds()

	dSegments := make([]diarize.Segment, 0, len(segments))
	for _, s := range segments {
		ds := diarize.Segment{Start: s.Start, End: s.End, Text: s.Text}
		for _, w := range s.Words {
			ds.Words = append(ds.Words, diarize.Word{Text: w.Text, Start: w.Start, End: w.End})
		}
		dSegments = append(dSegments, ds)
	}

	utterances, speakers := diarize.AssignSpeakers(dSegments, dres.Turns, diarize.MergeOptions{})
	text := providers.FormatPlainSegments(segments)
	note := ""
	if len(utterances) > 0 {
		text = diarize.Render(utterances, diarize.RenderOptions{Timestamps: cfg.Timestamps})
	} else {
		note = "Diarization produced no speaker turns; transcript is unlabeled."
	}

	dInfo := &schema.DiarizationInfo{
		Enabled:      true,
		Diarizer:     diarizer,
		Model:        diarizerModel,
		Device:       dres.Device,
		SpeakerCount: dres.SpeakerCount,
		Seconds:      dres.Seconds,
		Note:         note,
	}
	for _, s := range speakers {
		dInfo.Speakers = append(dInfo.Speakers, schema.SpeakerInfo{
			ID:            s.ID,
			Label:         s.Label,
			Turns:         s.Turns,
			SpeechSeconds: s.SpeechSeconds,
		})
	}
	if dInfo.SpeakerCount == 0 {
		dInfo.SpeakerCount = len(speakers)
	}

	pp.TotalSeconds = time.Since(startAll).Seconds()
	return text, pp, dInfo, nil
}

func cachedDiarizationInfo(provider, diarizer, model string, enabled bool) *schema.DiarizationInfo {
	if diarizer != "" {
		return &schema.DiarizationInfo{
			Enabled:  true,
			Diarizer: diarizer,
			Model:    model,
		}
	}
	if (provider == "deepgram" || provider == "assemblyai") && enabled {
		return &schema.DiarizationInfo{
			Enabled:  true,
			Diarizer: provider,
			Model:    model,
		}
	}
	return nil
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
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
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

func runDiarizerSetup(ctx context.Context, cfg Config) (schema.Response, error) {
	cacheDir, err := diarize.DefaultCacheDir()
	if err != nil {
		return schema.ErrorResponse(schema.ErrProviderError, "Failed to resolve cache dir.", err.Error()), nil
	}
	action := strings.ToLower(strings.TrimSpace(cfg.DiarizerAction))
	if action == "" {
		action = "status"
	}

	model := strings.TrimSpace(cfg.DiarizerModel)
	if model == "" {
		model = diarize.DefaultModel
	}
	hfConfigured := resolveHFToken(cfg) != ""

	switch action {
	case "install":
		st, err := diarize.Install(ctx, diarize.InstallOptions{
			PythonSpec: cfg.DiarizerPython,
			Force:      cfg.DiarizerForce,
			Verbose:    cfg.Verbose,
			Stderr:     os.Stderr,
			CacheDir:   cacheDir,
		})
		if err != nil {
			return schema.ErrorResponse(schema.ErrLocalDependencyMissing, "Failed to install diarization runtime.", err.Error()), nil
		}
		return schema.Response{
			OK:   true,
			Mode: "diarizer_setup",
			LocalEnv: localEnvInfo(st, model, cfg.DiarizerDevice, hfConfigured,
				"pyannote diarization runtime installed."),
			Note:    "Install the Hugging Face token and accept the model terms before running diarization.",
			Version: Version,
		}, nil
	case "uninstall", "remove":
		st := diarize.EnvStatusFor(cacheDir)
		if err := os.RemoveAll(st.VenvPath); err != nil {
			return schema.ErrorResponse(schema.ErrLocalDependencyMissing, "Failed to remove diarization runtime.", err.Error()), nil
		}
		return schema.Response{
			OK:   true,
			Mode: "diarizer_setup",
			LocalEnv: localEnvInfo(diarize.EnvStatusFor(cacheDir), model, cfg.DiarizerDevice, hfConfigured,
				"pyannote diarization runtime removed."),
			Version: Version,
		}, nil
	case "status":
		st := diarize.EnvStatusFor(cacheDir)
		note := "Diarization runtime is ready."
		if !st.Installed {
			note = "Diarization runtime is not installed. Run: memos2txt diarizer install"
		}
		return schema.Response{
			OK:       true,
			Mode:     "diarizer_setup",
			LocalEnv: localEnvInfo(st, model, cfg.DiarizerDevice, hfConfigured, note),
			Version:  Version,
		}, nil
	default:
		return schema.ErrorResponse(schema.ErrInvalidArgs, "Unsupported diarizer action.", "action="+action+" (supported: install, status, uninstall)"), nil
	}
}

func localEnvInfo(st diarize.EnvStatus, model, device string, hfConfigured bool, note string) *schema.LocalEnvInfo {
	if strings.TrimSpace(device) == "" {
		device = "auto"
	}
	return &schema.LocalEnvInfo{
		Kind:            diarize.EnvKind,
		Installed:       st.Installed,
		VenvPath:        st.VenvPath,
		Python:          st.Python,
		PythonVersion:   st.PythonVersion,
		PyannoteVersion: st.PyannoteVersion,
		Model:           model,
		Device:          device,
		HFConfigured:    hfConfigured,
		Note:            note,
	}
}

func credentialStatus(provider string, store secrets.Store) string {
	if providers.IsLocal(provider) {
		return "not required (local)"
	}
	envVar, ok := providers.EnvVarForProvider(provider)
	if !ok {
		return "not configured"
	}
	if value, used := lookupCredentialEnv(envVar); value != "" {
		return "configured (env: " + used + ")"
	}
	if store != nil {
		if v, err := store.Get(envVar); err == nil && strings.TrimSpace(v) != "" {
			return "configured (store: " + store.Kind() + ")"
		}
	}
	return "not configured"
}

func lookupCredential(envVar, storeKind string) string {
	if value, _ := lookupCredentialEnv(envVar); value != "" {
		return value
	}
	store, err := secrets.NewStore(storeKind, "memos2txt")
	if err != nil {
		return ""
	}
	if v, err := store.Get(envVar); err == nil {
		return strings.TrimSpace(v)
	}
	return ""
}

func lookupCredentialEnv(envVar string) (string, string) {
	names := []string{envVar}
	if envVar == "HF_TOKEN" {
		names = append(names, "HUGGING_FACE_HUB_TOKEN", "HUGGINGFACE_TOKEN")
	}
	for _, name := range names {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v, name
		}
	}
	return "", ""
}

func resolveHFToken(cfg Config) string {
	return lookupCredential("HF_TOKEN", resolveStore(cfg.Store))
}

func isLocalModelPath(model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	if strings.HasPrefix(model, ".") || strings.HasPrefix(model, "/") || strings.HasPrefix(model, "~") {
		return true
	}
	if filepath.IsAbs(model) {
		return true
	}
	_, err := os.Stat(model)
	return err == nil
}

func isDecodableAudio(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".wav", ".mp3", ".m4a", ".mp4", ".caf", ".flac", ".aiff", ".aif", ".ogg", ".opus", ".webm":
		return true
	default:
		return false
	}
}

func conditionalPath(path string, keep bool) string {
	if keep {
		return path
	}
	return ""
}

func cleanupTemp(path string, keep bool) {
	if !keep {
		_ = os.RemoveAll(path)
	}
}

func joinNotes(notes ...string) string {
	parts := make([]string, 0, len(notes))
	for _, note := range notes {
		if strings.TrimSpace(note) != "" {
			parts = append(parts, note)
		}
	}
	return strings.Join(parts, " ")
}

func fileSizeBytes(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

func mapAnyError(err error) (schema.ErrorCode, string, string) {
	if code, msg, detail, ok := diarize.MapError(err); ok {
		return code, msg, detail
	}
	return providers.MapProviderError(err)
}
