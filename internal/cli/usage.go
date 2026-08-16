package cli

func HelpText() string {
	return `memos2txt — Apple Voice Memos → Groq Whisper → JSON transcript

USAGE
  memos2txt --provider groq|deepgram|assemblyai --file "/path/to/audio.m4a" [flags]
  memos2txt auth <adapter> [flags]
  memos2txt auth --list

AUTHENTICATION COMMAND
  Set API key for an adapter (groq | deepgram | assemblyai):
    memos2txt auth groq
    memos2txt auth deepgram
    memos2txt auth assemblyai

  Non-interactive (recommended for scripts / CI):
    echo "$GROQ_API_KEY" | memos2txt auth groq --api-key-stdin

  Delete stored key:
    memos2txt auth groq --unset

  Check status of all adapters:
    memos2txt auth --list

  Storage backend:
    --store keychain|file (default: keychain on macOS, file ~/.memos2txt/config.json on Linux)

MACOS PERMISSIONS NOTE

RUNTIME API KEY LOOKUP ORDER
  1) Environment variable (e.g. GROQ_API_KEY)
  2) Configured store (--store keychain|file; default keychain on macOS)

NOTES
  - Normal runs write JSON to stdout only. Logs go to stderr.
  - Transcript caching is content-hash based (sha256) and prevents re-transcribing the same audio.

EXAMPLES
  Direct file:
    memos2txt --provider groq --file "/path/to/audio.m4a" --json
    memos2txt --provider deepgram --file "/path/to/audio.m4a" --json

  Disable cache:
    memos2txt --provider groq --file "/path/to/audio.m4a" --no-cache

FLAGS (selected)
  Input:
    --file <path>                Direct audio file path (required)

  Provider:
    --provider groq|deepgram|assemblyai
    --model <string>              Defaults: groq=whisper-large-v3, deepgram=nova-3, assemblyai=best
    --language auto|en|zh|...
    --timeout <seconds>           Default 300
    --diarization true|false      Deepgram default: true (use --diarization=false to disable)
    --punctuate true|false

  Output:
    --json=true|false             Default true
    --inline-max-chars N          Default 4000
    --preview-lines N             Default 5
    --quiet
    --verbose

  Cache:
    --no-cache
    --max-file-mb N               Default 200

  Preprocess (ffmpeg):
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

ENV VARS
  GROQ_API_KEY
  DEEPGRAM_API_KEY
  ASSEMBLYAI_API_KEY

PROVIDER LIMITS
  Groq may reject large uploads with HTTP 413 (Request Entity Too Large). If that happens,
  memos2txt can auto-preprocess (ffmpeg re-encode + chunk) to work around it.

EXIT CODES
  0 success
  1 JSON error (see error.code)
  2 invalid CLI args (JSON error)
`
}
