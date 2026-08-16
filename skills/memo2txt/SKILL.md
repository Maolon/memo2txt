---
name: memo2txt
description: Transcribe local audio files and Apple Voice Memos (.m4a, .caf, .mp3, .wav) into clean JSON transcripts using cloud ASR providers (Groq Whisper, Deepgram Nova-3, AssemblyAI) with deterministic content-hash caching and secure credentials. Use when the user asks to transcribe voice memos, audio recordings, meeting notes, speech-to-text, or manage speech API keys. Triggers include "transcribe audio", "transcribe memo", "voice memo to text", "memos2txt", "memo2txt", "m4a transcript", "speech to text", "audio transcription".
---

# memo2txt

Transcribe local audio files and Apple Voice Memos (.m4a, .caf, .mp3, .wav) to structured JSON with cloud ASR (Groq, Deepgram, AssemblyAI). Transcripts are deduplicated and cached automatically by audio content hash.

## Quick reference

```bash
# Point to the compiled memos2txt binary
MEMOS2TXT="memos2txt" # or /path/to/memo2txt/bin/memos2txt

# 1. Manage API Keys
$MEMOS2TXT auth --list
echo "$GROQ_API_KEY" | $MEMOS2TXT auth groq --api-key-stdin
echo "$DEEPGRAM_API_KEY" | $MEMOS2TXT auth deepgram --api-key-stdin
echo "$ASSEMBLYAI_API_KEY" | $MEMOS2TXT auth assemblyai --api-key-stdin
$MEMOS2TXT auth groq --unset

# 2. Transcribe Audio File
$MEMOS2TXT --provider groq --file "/path/to/audio.m4a" --json
$MEMOS2TXT --provider deepgram --file "/path/to/audio.m4a" --diarization=true --json
$MEMOS2TXT --provider assemblyai --file "/path/to/audio.m4a" --json

# 3. Force Re-transcription (Bypass Cache)
$MEMOS2TXT --provider groq --file "/path/to/audio.m4a" --no-cache --json
```

All standard runs emit **JSON only to stdout**. Diagnostics and logs go to stderr.

---

## Core Workflows

### 1. Authentication & Key Setup

Credentials are checked in this order:
1. Environment variables (`GROQ_API_KEY`, `DEEPGRAM_API_KEY`, `ASSEMBLYAI_API_KEY`)
2. System secret store (macOS Keychain by default; `~/.memos2txt/config.json` on Linux)

Check status of all adapters:
```bash
memos2txt auth --list
```

Add or update a provider key non-interactively:
```bash
echo "gsk_your_groq_api_key" | memos2txt auth groq --api-key-stdin
echo "your_deepgram_api_key" | memos2txt auth deepgram --api-key-stdin
```

Delete a stored key:
```bash
memos2txt auth groq --unset
```

---

### 2. Transcribing Audio

#### Standard Groq Whisper (Fastest, cost-efficient):
```bash
memos2txt --provider groq --file "/path/to/recording.m4a" --json
```

#### Deepgram Nova-3 with Speaker Diarization (Best for meetings/multi-speaker):
```bash
# Diarization is enabled by default for Deepgram
memos2txt --provider deepgram --file "/path/to/meeting.m4a" --json
```

#### AssemblyAI (Best for speech accuracy & formatting):
```bash
memos2txt --provider assemblyai --file "/path/to/lecture.m4a" --json
```

---

### 3. Locating Apple Voice Memos on macOS

Apple Voice Memos are stored in macOS Group Containers. Due to macOS TCC privacy protections, agents should check standard paths or advise users to export to `~/Downloads`:

Common local recording paths:
- `~/Library/Group Containers/group.com.apple.VoiceMemos.shared/Recordings/`
- `~/Library/Application Support/com.apple.voicememos/Recordings/`

Example shell discovery:
```bash
find ~/Library/Group\ Containers/group.com.apple.VoiceMemos.shared/Recordings -name "*.m4a" -type f
```

---

### 4. Handling Output JSON

The CLI emits a strict JSON response. Calling scripts and agents should parse stdout:

#### Short Transcript Response (`inline_mode: "full"`):
```json
{
  "ok": true,
  "cache_hit": false,
  "provider": "groq",
  "model": "whisper-large-v3",
  "options": { "language": "auto", "diarization": false, "punctuate": true },
  "input": {
    "file_path": "/path/to/audio.m4a",
    "file_size_bytes": 524288,
    "mtime_unix": 1700000000
  },
  "output": {
    "transcript_path": "/Users/user/.memos2txt/transcripts/<hash>.groq.whisper-large-v3.auto.d0.p1.txt",
    "inline_mode": "full",
    "transcript_full": "Hello world, this is the complete transcribed text.",
    "chars": 53,
    "lines": 1
  }
}
```

#### Long Transcript Response (`inline_mode: "preview"`):
When transcript length exceeds `--inline-max-chars` (default 4000), `transcript_full` is left empty to save context tokens. Read `transcript_path` directly to access full content:
```json
{
  "ok": true,
  "cache_hit": true,
  "output": {
    "transcript_path": "/Users/user/.memos2txt/transcripts/<hash>.deepgram.nova-3.auto.d1.p1.txt",
    "inline_mode": "preview",
    "transcript_preview": [
      "Speaker 0: Good morning everyone...",
      "Speaker 1: Thanks for joining today's sprint review."
    ],
    "chars": 45000,
    "lines": 350
  },
  "note": "Transcript too long; read transcript_path for full text."
}
```

---

### 5. Large Audio Preprocessing & Chunking

For audio files exceeding 25MB or 1 hour (3600s) duration:
- `memos2txt` automatically uses `ffprobe` to inspect duration.
- It normalizes the audio to mono 16kHz 32kbps MP3 via `ffmpeg`.
- It splits files >3600s into 600s chunks, transcribes sequentially, and concatenates results seamlessly.

---

### 6. Error Handling

If `ok` is `false`, inspect `error.code`:
- `FILE_NOT_FOUND`: Specified audio file does not exist.
- `FILE_UNREADABLE`: Missing read permissions on file.
- `API_KEY_MISSING`: Set API key via `memos2txt auth <provider>` or environment variable.
- `PROVIDER_ERROR`: Upstream cloud provider rejected request.
- `TIMEOUT`: Request exceeded timeout limit (increase with `--timeout <seconds>`).
- `INVALID_ARGS`: Missing required flags or invalid provider name.
