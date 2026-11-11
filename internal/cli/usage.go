package cli

func HelpText() string {
	return `memos2txt — local & cloud audio → deterministic JSON transcript

USAGE
  memos2txt --provider <id> --file "/path/to/audio.m4a" [flags]
  memos2txt auth <adapter> [flags]
  memos2txt auth --list
  memos2txt diarizer <install|status|uninstall> [flags]

PROVIDERS (category → id)
  cloud   groq | deepgram | assemblyai
  local   mlx-whisper   (Apple Silicon; requires the mlx_whisper CLI)

LOCAL SPEAKER DIARIZATION
  1) Install the managed pyannote runtime once (~1–2 GB of Python deps):
       memos2txt diarizer install
  2) Accept the model terms and store a Hugging Face token:
       https://huggingface.co/pyannote/speaker-diarization-community-1
       echo "$HF_TOKEN" | memos2txt auth huggingface --api-key-stdin
  3) Transcribe with speaker labels:
       memos2txt --provider mlx-whisper --file meeting.m4a --diarizer pyannote --json
       memos2txt --provider mlx-whisper --file meeting.m4a --diarizer pyannote --timestamps --json

AUTHENTICATION COMMAND
  Set an API key / token for an adapter:
    memos2txt auth groq
    memos2txt auth deepgram
    memos2txt auth assemblyai
    memos2txt auth huggingface

  Non-interactive (recommended for scripts / CI):
    echo "$GROQ_API_KEY" | memos2txt auth groq --api-key-stdin
    echo "$HF_TOKEN" | memos2txt auth huggingface --api-key-stdin

  Delete stored credential:
    memos2txt auth groq --unset

  Check status of all adapters:
    memos2txt auth --list

  Storage backend:
    --store keychain|file (default: keychain on macOS, file ~/.memos2txt/config.json on Linux)

RUNTIME CREDENTIAL LOOKUP ORDER
  1) Environment variable (e.g. GROQ_API_KEY, HF_TOKEN)
  2) Configured store (--store keychain|file; default keychain on macOS)

NOTES
  - Normal runs write JSON to stdout only. Logs go to stderr.
  - Transcript caching is content-hash based (sha256) and includes provider,
    model, language, diarization, diarizer, and diarizer model.
  - Local diarization uses pyannote/speaker-diarization-community-1 and assigns
    speakers from word-level Whisper timestamps. It needs ffmpeg.

EXAMPLES
  Cloud:
    memos2txt --provider groq --file "/path/to/audio.m4a" --json
    memos2txt --provider deepgram --file "/path/to/audio.m4a" --json

  Local (Apple Silicon):
    memos2txt --provider mlx-whisper --file "/path/to/audio.m4a" --json
    memos2txt --provider mlx-whisper --file "/path/to/audio.m4a" \
      --model mlx-community/whisper-large-v3-turbo --json

  Local with speakers:
    memos2txt --provider mlx-whisper --file "/path/to/meeting.m4a" \
      --diarizer pyannote --num-speakers 3 --timestamps --json

  Disable cache:
    memos2txt --provider mlx-whisper --file "/path/to/audio.m4a" --no-cache

FLAGS (selected)
  Input:
    --file <path>                Direct audio file path (required)

  Provider:
    --provider groq|deepgram|assemblyai|mlx-whisper
    --model <string>              Defaults: groq=whisper-large-v3, deepgram=nova-3,
                                  assemblyai=best, mlx-whisper=mlx-community/whisper-large-v3-turbo
    --language auto|en|zh|...
    --timeout <seconds>           Default 300; local providers default 3600
    --diarization true|false      Deepgram defaults to true; local uses --diarizer
    --punctuate true|false

  Local diarization (provider=mlx-whisper):
    --diarizer auto|none|pyannote Default auto (pyannote when --diarization=true)
    --diarizer-model <repo>       Default pyannote/speaker-diarization-community-1
    --diarizer-device auto|cpu|mps|cuda
    --num-speakers N              Exact speaker count (0 = infer)
    --min-speakers N              Minimum speakers (0 = infer)
    --max-speakers N              Maximum speakers (0 = infer)
    --timestamps                  Prefix lines with [MM:SS]
    --whisper-prompt <text>       Initial prompt for names/jargon

  Output:
    --json=true|false             Default true
    --inline-max-chars N          Default 4000
    --preview-lines N             Default 5
    --quiet
    --verbose

  Cache:
    --no-cache
    --max-file-mb N               Default 200

  Preprocess (ffmpeg, cloud providers):
    --auto-preprocess=true|false  Default true
    --preprocess-threshold-mb N   Default 25
    --chunk-seconds N             0=auto (probe, chunk if >1h); >0 forces chunking at N
    --preprocess-bitrate-kbps N   Default 32
    --preprocess-sample-rate-hz N Default 16000
    --keep-temp                   Keep temp chunks

  Secrets:
    --store keychain|file         Default: keychain on macOS
    --setup
    --unset-api-key
    --api-key <value>
    --api-key-stdin

DIARIZER COMMAND
  memos2txt diarizer install [--python 3.12] [--force]
  memos2txt diarizer status
  memos2txt diarizer uninstall

ENV VARS
  GROQ_API_KEY
  DEEPGRAM_API_KEY
  ASSEMBLYAI_API_KEY
  HF_TOKEN                         (aliases: HUGGING_FACE_HUB_TOKEN, HUGGINGFACE_TOKEN)
  MEMOS2TXT_MLX_WHISPER_BIN        Override the mlx_whisper executable
  MEMOS2TXT_UV_BIN                 Override the uv executable used by diarizer install

PROVIDER LIMITS
  Groq may reject large uploads with HTTP 413 (Request Entity Too Large). If that happens,
  memos2txt can auto-preprocess (ffmpeg re-encode + chunk) to work around it.
  Local providers do not use --chunk-seconds; mlx-whisper handles long files internally.

EXIT CODES
  0 success
  1 JSON error (see error.code)
  2 invalid CLI args (JSON error)
`
}
