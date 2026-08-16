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

## API key setup (inside the CLI; no subcommands)

Recommended on macOS: store in **Keychain** (default store).

Interactive (recommended):

```bash
memos2txt --setup --provider groq
```

Non-interactive (recommended for scripts):

```bash
echo "$GROQ_API_KEY" | memos2txt --setup --provider groq --api-key-stdin
```

Remove stored key:

```bash
memos2txt --setup --provider groq --unset-api-key
```

Runtime lookup order:

1. `GROQ_API_KEY` environment variable
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

## Docs

- Proposal: `docs/purposal.md`
- Technical doc: `docs/technical.md`
