<div align="center">

# memos2txt

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Language](https://img.shields.io/badge/language-Go_1.22+-00ADD8.svg)](https://go.dev/)
[![Platform](https://img.shields.io/badge/platform-macOS_|_Linux_|_Windows-lightgrey.svg)](#-cross-platform-support)
[![Output](https://img.shields.io/badge/stdout-JSON_only-green.svg)](#-output-contract)
[![Cache](https://img.shields.io/badge/cache-SHA256_content_hash-orange.svg)](#-content-hash-caching)

**Audio Files → Cloud or Local ASR → Deterministic JSON Transcript**

**Cloud:** Groq / Deepgram / AssemblyAI · **Local:** mlx-whisper (Apple Silicon) + pyannote speaker diarization

[Key Features](#-key-features) • [Performance](#-measured-performance) • [Why memos2txt](#️-why-memos2txt) • [Quickstart](#-quickstart) • [Authentication](#-authentication) • [Output Contract](#-output-contract) • [Commands](#️-command-reference)

</div>

---

## ⚡ Measured Performance

| Metric | Measured Result |
| --- | --- |
| **Download Size** | **2.3–2.5 MiB** compressed distribution archive (`.tar.gz` / `.zip`). |
| **Binary Footprint** | **5.9–6.6 MiB** single static Go executable (`-ldflags="-s -w"`), zero runtime CGo dependencies. |
| **Memory Footprint** | Peak RSS **11–14 MiB** (streaming SHA256 calculation & streaming multipart upload). |
| **Cache Hit Latency** | **16–20 ms** local NVMe disk retrieval (0 remote network round-trips). |
| **Transcription Latency** | **~400–600 ms** single-shot upload & ASR response via Groq Whisper API. |
| **Agent Efficiency** | **1 tool call** per task; stdout is 100% strict machine-parseable JSON. |

*Measured on Apple Silicon M-series (macOS 15.x) using Groq Whisper Large-v3 and Deepgram Nova-3 with Go 1.23.*

---

## 🌟 Key Features

### 1. 🎙️ Multi-Provider ASR: Cloud + Local
Connect local `.m4a`, `.caf`, `.mp3`, and `.wav` audio files directly to industry-leading speech-to-text models:
* **Groq Whisper (`whisper-large-v3`)** — Cloud. Ultra-fast, cost-effective transcription.
* **Deepgram (`nova-3`)** — Cloud. Production-grade recognition with **speaker diarization** and smart formatting.
* **AssemblyAI (`best`)** — Cloud. High-accuracy transcription with automated punctuation and speaker labeling.
* **mlx-whisper** — **Local category, Apple Silicon.** Runs Whisper locally through MLX, no audio leaves the machine. Uses word timestamps for downstream speaker assignment.

Provider taxonomy is explicit: `option.category` is `"cloud"` or `"local"`; the provider id for local ASR is `mlx-whisper` (not `local`).

### 2. 🗣️ Local Speaker Diarization (pyannote community-1)
Run the full local labeling pipeline — one provider operation, equivalent to Deepgram doing transcription + diarization together:
```bash
memos2txt diarizer install
memos2txt --provider mlx-whisper --file meeting.m4a --diarization=true --json
```
* Uses `pyannote/speaker-diarization-community-1` internally in a managed Python environment (`~/.memos2txt/envs/pyannote`).
* Assigns speakers from word-level Whisper timestamps with overlap-aware turns and short-run smoothing.
* Outputs Deepgram-compatible `[Speaker:N]` labels plus a deterministic `diarization` block with speaker counts and speech seconds.
* When `--diarization` is omitted, local diarization auto-enables if the runtime and HF token are ready; otherwise local transcription runs plain with an explanatory note. Use `--diarization=false` to force plain transcription.
* Requires `HF_TOKEN` (model is gated) and `ffmpeg`. Accept the model terms on Hugging Face first.

### 3. ⚡ Deterministic Content-Hash Caching
Never pay for or wait on duplicate transcriptions:
* Keyed by `sha256(audio_bytes) + provider + model + language + diarization + punctuate + diarizer + diarizer_model`.
* Survives file renames and moves.
* Caches stored in `~/.memos2txt/transcripts/` with strict `0600` file permissions.

### 4. ✂️ Smart Audio Preprocessing & Adaptive Chunking
Transparently overcomes provider upload size limits and timeout constraints:
* **Auto-Probing**: Automatically checks audio duration via `ffprobe`.
* **Adaptive Chunking**: Files exceeding 1 hour (3600s) or triggering HTTP 413 are automatically normalized to 16kHz mono MP3 via `ffmpeg`, split into 600s chunks, transcribed sequentially, and merged.

### 5. 🤖 Agent-First JSON-Only Contract
Designed from the ground up for LLM coding agents (Claude Code, Cursor, Codex, Pi) and shell pipelines:
* **stdout is 100% JSON**: Success and error payloads both emit standard JSON objects.
* **stderr for Diagnostics**: Human-facing progress, prompts, and debug logs are routed strictly to stderr.
* **Inline vs. Preview Policy**: Automatically inlines full transcript for short audio (`inline_mode: "full"`), and yields line previews plus file path for long audio (`inline_mode: "preview"`) to prevent agent context overflow.

### 6. 🔑 Unified Multi-Platform Authentication (`auth <adapter>`)
* Secure native credential storage: macOS **Keychain**, Windows **Credential Manager**, and Linux configuration file (`~/.memos2txt/config.json`).
* Seamless fallback to environment variables (`GROQ_API_KEY`, `DEEPGRAM_API_KEY`, `ASSEMBLYAI_API_KEY`, `HF_TOKEN`).
* Hugging Face is a model-hub adapter (`memos2txt auth huggingface`) used by local diarization.

---

## ⚖️ Why memos2txt?

| Feature | Local Whisper (`whisper.cpp` / MLX) | Python Scripts (`openai-whisper`) | Cloud Web Consoles | memos2txt |
| --- | --- | --- | --- | --- |
| **Cloud footprint** | n/a | n/a | Manual drag-and-drop | ✅ **Lightweight (~2.4MB archive / ~6MB binary)** |
| **Local privacy** | Audio stays local | Audio stays local | Audio uploaded | ✅ **`mlx-whisper` + pyannote stay local** |
| **Agent Friendly** | Mixed stdout logs and stderr noise | Unstable stdout formatting | No terminal / CLI interface | ✅ **Deterministic stdout JSON only** |
| **Deduplication** | Manual caching logic needed | Re-transcribes every run | No automated caching | ✅ **Automatic SHA256 content cache** |
| **Large Audio Handling** | Manual chunking | Complex chunking boilerplate | Hard file size ceilings | ✅ **Automated probe & ffmpeg chunking (cloud)** |
| **Multi-Provider** | Locked to Whisper weights | Separate SDKs for each vendor | Fragmented web consoles | ✅ **Groq + Deepgram + AssemblyAI + mlx-whisper unified** |
| **Speaker labels** | Not built in | Manual pyannote assembly | Varies by vendor | ✅ **Deepgram native + local pyannote community-1** |

---

## 🚀 Quickstart

### 🤖 For AI Coding Agents

Install the agent skill via the standard `skills` CLI:

```bash
# Standard npx skills convention (Claude Code, Codex, Pi, Cursor)
npx -y skills add Maolon/memo2txt
```

*Or manually copy the skill files into your agent's skill directory (`~/.claude/skills/memo2txt` or `~/.codex/skills/memo2txt`).*

---

### 👤 For Humans

#### Option A: One-Line Curl Installer (Recommended for macOS & Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/Maolon/memo2txt/main/install.sh | bash
```

#### Option B: Homebrew Tap

```bash
brew install Maolon/tap/memos2txt
```

#### Option C: Go Install (Build from Source)

```bash
go install github.com/Maolon/memo2txt/cmd/memos2txt@latest
```

*Prebuilt standalone binaries for Windows, macOS, and Linux are also directly downloadable from [GitHub Releases](https://github.com/Maolon/memo2txt/releases/latest).*

---

### 🔑 Configure Credentials & Run

```bash
# Cloud: store API keys in native Keychain / Credential Manager
echo "gsk_your_groq_key" | memos2txt auth groq --api-key-stdin
echo "your_deepgram_key" | memos2txt auth deepgram --api-key-stdin

# Check configuration status
memos2txt auth --list

# Cloud transcription
memos2txt --provider groq --file "/path/to/meeting.m4a" --json
memos2txt --provider deepgram --file "/path/to/interview.m4a" --json

# Local transcription (Apple Silicon)
memos2txt --provider mlx-whisper --file "/path/to/meeting.m4a" --json
```

### 🗣️ Local Speaker Labeling (one-time setup)

```bash
# 1. Install the managed pyannote environment (downloads torch + pyannote)
memos2txt diarizer install

# 2. Accept terms at https://huggingface.co/pyannote/speaker-diarization-community-1
#    then store the token inside the OS credential store
echo "$HF_TOKEN" | memos2txt auth huggingface --api-key-stdin

# 3. Transcribe + diarize, fully local (same provider pipeline as Deepgram)
memos2txt --provider mlx-whisper --file "/path/to/meeting.m4a" --diarization=true --json

# Optional: timestamps, known speaker count, CPU-only
memos2txt --provider mlx-whisper --file "/path/to/meeting.m4a" \
  --diarization=true --num-speakers 3 --timestamps --json
memos2txt --provider mlx-whisper --file "/path/to/meeting.m4a" \
  --diarization=true --diarization-device cpu --json

# Force plain local transcription (no speaker labels)
memos2txt --provider mlx-whisper --file "/path/to/meeting.m4a" --diarization=false --json

# Runtime status / removal
memos2txt diarizer status
memos2txt diarizer uninstall
```

### 🧠 MLX Whisper Model Options (large-v3-turbo variants)

Yes — `whisper-large-v3-turbo` has MLX conversions maintained by `mlx-community`. `memos2txt` accepts any MLX repo id or local model directory through `--model`:

| MLX repo | Approx. weights | Recommended when |
| --- | --- | --- |
| `mlx-community/whisper-large-v3-turbo` | ~1.61 GB (fp16) | Default. Best accuracy, enough disk. |
| `mlx-community/whisper-large-v3-turbo-8bit` | ~863 MB | Balanced disk/accuracy. |
| `mlx-community/whisper-large-v3-turbo-4bit` | ~463 MB | Low disk / laptop-friendly. |
| `mlx-community/whisper-large-v3-turbo-asr-fp16` | ~1.61 GB | ASR-focused conversion. |
| `mlx-community/whisper-large-v3-turbo-asr-4bit` | ~463 MB | ASR-focused + low disk. |
| `mlx-community/belle-whisper-large-v3-turbo-zh-8bit` | ~863 MB | Chinese fine-tune. |

```bash
# Low-disk variant (~463 MB instead of ~1.6 GB)
memos2txt --provider mlx-whisper \
  --model mlx-community/whisper-large-v3-turbo-4bit \
  --file meeting.m4a --json
```

Models are cached by Hugging Face under `~/.cache/huggingface/hub/`. The first run downloads the weights; later runs are offline-capable. Quantized 4bit/8bit repos that ship `model.safetensors` are supported automatically: `memos2txt` adds a cache-local `weights.safetensors` alias for mlx-whisper 0.4.3.

---

## 🛠️ Command Reference

### Authentication Commands

```bash
# Interactive setup (masked password entry)
memos2txt auth groq
memos2txt auth deepgram
memos2txt auth assemblyai
memos2txt auth huggingface   # HF_TOKEN for gated pyannote models

# Non-interactive / CI setup
echo "$GROQ_API_KEY" | memos2txt auth groq --api-key-stdin
echo "$HF_TOKEN" | memos2txt auth huggingface --api-key-stdin

# List authentication status across all adapters
memos2txt auth --list

# Delete stored credential
memos2txt auth groq --unset
```

### Transcription Flags

```bash
memos2txt --provider <groq|deepgram|assemblyai|mlx-whisper> --file <path> [flags]
```

| Category | Provider id | Where it runs | Default model |
| --- | --- | --- | --- |
| cloud | `groq` | Groq API | `whisper-large-v3` |
| cloud | `deepgram` | Deepgram API | `nova-3` |
| cloud | `assemblyai` | AssemblyAI API | `best` |
| local | `mlx-whisper` | Apple Silicon (MLX) | `mlx-community/whisper-large-v3-turbo` |

| Flag | Default | Description |
| --- | --- | --- |
| `--file <path>` | *(required)* | Path to local audio file (`.m4a`, `.caf`, `.mp3`, `.wav`). |
| `--provider <id>` | *(required)* | Cloud: `groq`, `deepgram`, `assemblyai`. Local: `mlx-whisper`. |
| `--model <string>` | Provider default | Model override (cloud model id or MLX HF repo / local path). |
| `--language <string>` | `auto` | Language hint (`en`, `zh`, `es`, etc.). |
| `--diarization` | `auto` *(local)* | Run transcription + speaker labeling as one pipeline. Local auto-enables when runtime/token are ready; Deepgram defaults true. |
| `--diarization-model <repo>` | `pyannote/speaker-diarization-community-1` | pyannote pipeline override (local). |
| `--diarization-device <dev>` | `auto` | `auto`, `cpu`, `mps`, or `cuda` (local). GPU falls back to CPU on failure. |
| `--num-speakers N` | `0` *(infer)* | Exact speaker count. |
| `--min-speakers N` / `--max-speakers N` | `0` | Speaker count bounds. |
| `--timestamps` | `false` | Prefix diarized lines with `[MM:SS]`. |
| `--whisper-prompt <text>` | *(empty)* | Initial prompt for names/jargon (local Whisper). |
| `--punctuate` | `true` | Enable smart punctuation and formatting. |
| `--no-cache` | `false` | Force re-transcription by bypassing disk cache. |
| `--inline-max-chars N` | `4000` | Inline full text if chars <= N; otherwise return preview. |
| `--preview-lines N` | `5` | Number of preview lines returned when transcript is long. |
| `--auto-preprocess` | `true` | Cloud only. Normalize audio via ffmpeg if >25MB. |
| `--chunk-seconds N` | `0` *(auto)* | Cloud only. `0` = auto-probe; `>0` forces chunking. |
| `--timeout <sec>` | `300` (`3600` local) | Request/run timeout in seconds. |

### Diarizer Runtime Commands

```bash
memos2txt diarizer install [--python 3.12] [--force]
memos2txt diarizer status
memos2txt diarizer uninstall
```

---

## 📋 Output Contract

### Success JSON Response (Short Transcript)

```json
{
  "ok": true,
  "cache_hit": false,
  "category": "local",
  "provider": "mlx-whisper",
  "model": "mlx-community/whisper-large-v3-turbo",
  "mode": "transcribe",
  "options": {
    "language": "auto",
    "punctuate": true,
    "diarization": true,
    "diarizer": "pyannote",
    "diarizer_model": "pyannote/speaker-diarization-community-1",
    "timestamps": true
  },
  "input": {
    "file_path": "/path/to/audio.m4a",
    "file_size_bytes": 524288,
    "mtime_unix": 1700000000
  },
  "output": {
    "transcript_path": "/Users/user/.memos2txt/transcripts/abc123hash.mlx-whisper.mlx-community-whisper-large-v3-turbo.auto.d1.p1.pyannote.pyannote-speaker-diarization-community-1.txt",
    "inline_mode": "full",
    "transcript_full": "[00:00] [Speaker:0] Good morning. Here is the full transcription of our discussion.\n[00:42] [Speaker:1] Thank you.",
    "chars": 90,
    "lines": 2
  },
  "diarization": {
    "enabled": true,
    "diarizer": "pyannote",
    "model": "pyannote/speaker-diarization-community-1",
    "device": "mps",
    "speaker_count": 2,
    "speakers": [
      { "id": "SPEAKER_00", "label": "Speaker 0", "turns": 1, "speech_seconds": 12.4 },
      { "id": "SPEAKER_01", "label": "Speaker 1", "turns": 1, "speech_seconds": 4.1 }
    ],
    "seconds": 18.7
  },
  "version": "0.2.0"
}
```

### Success JSON Response (Long Transcript with Preview)

```json
{
  "ok": true,
  "cache_hit": true,
  "category": "cloud",
  "provider": "deepgram",
  "model": "nova-3",
  "options": { "language": "auto", "diarization": true, "punctuate": true },
  "output": {
    "transcript_path": "/Users/user/.memos2txt/transcripts/xyz987hash.deepgram.nova-3.auto.d1.p1.txt",
    "inline_mode": "preview",
    "transcript_preview": [
      "[Speaker:0] Welcome to the quarterly financial review.",
      "[Speaker:1] Thank you. Let's start with revenue numbers..."
    ],
    "chars": 52000,
    "lines": 420
  },
  "diarization": { "enabled": true, "diarizer": "deepgram", "model": "nova-3" },
  "note": "Transcript too long; read transcript_path for full text.",
  "version": "0.2.0"
}
```

### Error JSON Response

```json
{
  "ok": false,
  "error": {
    "code": "API_KEY_MISSING",
    "message": "Missing API key.",
    "detail": "Expected env var GROQ_API_KEY or configured store."
  }
}
```

*Full JSON Schema definition available at [`docs/output.schema.json`](docs/output.schema.json).*

---

## 💻 Cross-Platform Support

`memos2txt` is written in pure Go (`CGO_ENABLED=0`) and natively runs on:
* **macOS** (`darwin/arm64`, `darwin/amd64`): Native Apple Keychain integration. Local ASR (`mlx-whisper`) requires **Apple Silicon**; local diarization also works on Intel macOS.
* **Windows** (`windows/amd64`, `windows/arm64`): Native Windows Credential Manager integration. Cloud providers only.
* **Linux** (`linux/amd64`, `linux/arm64`): Config store at `~/.memos2txt/config.json`. Cloud providers; local diarization works when the managed pyannote environment is installed.

Local diarization depends on external runtimes: `ffmpeg`, `uv` (recommended) or Python 3.12+, and `mlx_whisper` for local ASR. These are never bundled into the static binary.

---

## 📄 License

MIT
