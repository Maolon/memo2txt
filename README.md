# memos2txt

Apple Voice Memos (`.m4a`) → Cloud ASR (Groq/Deepgram/AssemblyAI) → stable JSON transcript (agent-friendly, stateless).

## What it does

- Transcribes a local audio file given an explicit file path.
- Uploads to Groq Whisper for transcription.
- Writes a transcript cache file to `~/.memos2txt/transcripts/` keyed by content hash + options.
- Prints **JSON only** to stdout (errors included).

## Install

```bash
go build -o bin/memos2txt ./cmd/memos2txt
```

## Authentication (`memos2txt auth <adapter>`)

Manage API keys in the environment store (**Keychain** by default on macOS, `~/.memos2txt/config.json` on Linux):

```bash
# Interactive setup (input hidden)
memos2txt auth groq
memos2txt auth deepgram
memos2txt auth assemblyai

# Non-interactive / CI (from stdin)
echo "$GROQ_API_KEY" | memos2txt auth groq --api-key-stdin

# Check status of all adapters
memos2txt auth --list

# Remove stored key
memos2txt auth groq --unset
```

Runtime lookup order:
1. Environment variable (e.g. `GROQ_API_KEY`, `DEEPGRAM_API_KEY`, `ASSEMBLYAI_API_KEY`)
2. Configured store (`--store keychain|file`)

## Usage

macOS note: the default Voice Memos recording directory may be blocked by macOS privacy (TCC).
If so, export a memo to a normal folder (e.g. `~/Downloads`) and use `--file`.

Direct file mode:

```bash
memos2txt --provider groq --file "/path/to/audio.m4a" --json
```

Deepgram:

```bash
memos2txt --provider deepgram --file "/path/to/audio.m4a" --json
```

AssemblyAI:

```bash
memos2txt --provider assemblyai --file "/path/to/audio.m4a" --json
```

Help:

```bash
memos2txt -h
```

## Output contract

- stdout: JSON only
- stderr: logs only (no transcript text unless you choose to print it yourself)

Schema: `docs/output.schema.json`

## Cache / dedup

Cache key:

`sha256(file_bytes) + provider + model + language + diarization + punctuate`

Default location:

`~/.memos2txt/transcripts/<sha256>.<provider>.<model>.<lang>.d?.p?.txt`

## Schema

- Output Schema: `docs/output.schema.json`
