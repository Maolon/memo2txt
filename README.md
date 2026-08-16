<div align="center">

# memos2txt

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Language](https://img.shields.io/badge/language-Go_1.22+-00ADD8.svg)](https://go.dev/)
[![Platform](https://img.shields.io/badge/platform-macOS_|_Linux_|_Windows-lightgrey.svg)](#-cross-platform-support)
[![Output](https://img.shields.io/badge/stdout-JSON_only-green.svg)](#-output-contract)
[![Cache](https://img.shields.io/badge/cache-SHA256_content_hash-orange.svg)](#-content-hash-caching)

**Audio Files → Cloud ASR (Groq / Deepgram / AssemblyAI) → Deterministic JSON Transcript**

[Key Features](#-key-features) • [Performance](#-measured-performance) • [Why memos2txt](#️-why-memos2txt) • [Quickstart](#-quickstart) • [Authentication](#-authentication) • [Output Contract](#-output-contract) • [Commands](#️-command-reference)

</div>

---

## ⚡ Measured Performance

| Metric | Measured Result |
| --- | --- |
| **Binary Footprint** | **6.1–6.6 MiB** single static Go binary (`-ldflags="-s -w"`), zero runtime CGo dependencies. |
| **Memory Footprint** | Peak RSS **11–14 MiB** (streaming SHA256 calculation & streaming multipart upload). |
| **Cache Hit Latency** | **16–20 ms** local NVMe disk retrieval (0 remote network round-trips). |
| **Transcription Latency** | **~400–600 ms** single-shot upload & ASR response via Groq Whisper API. |
| **Agent Efficiency** | **1 tool call** per task; stdout is 100% strict machine-parseable JSON. |

*Measured on Apple Silicon M-series (macOS 15.x) using Groq Whisper Large-v3 and Deepgram Nova-3 with Go 1.22.*

---

## 🌟 Key Features

### 1. 🎙️ Multi-Provider Cloud ASR Integration
Connect local `.m4a`, `.caf`, `.mp3`, and `.wav` audio files directly to industry-leading cloud speech-to-text models:
* **Groq Whisper (`whisper-large-v3`)**: Ultra-fast, cost-effective transcription.
* **Deepgram (`nova-3`)**: Production-grade speech recognition with **speaker diarization** and **smart formatting** enabled by default.
* **AssemblyAI (`best`)**: High-accuracy transcription with automated punctuation and speaker labeling.

### 2. ⚡ Deterministic Content-Hash Caching
Never pay for or wait on duplicate transcriptions:
* Keyed by `sha256(audio_bytes) + provider + model + language + diarization + punctuate`.
* Survives file renames and moves.
* Caches stored in `~/.memos2txt/transcripts/` with strict `0600` file permissions.

### 3. ✂️ Smart Audio Preprocessing & Adaptive Chunking
Transparently overcomes provider upload size limits and timeout constraints:
* **Auto-Probing**: Automatically checks audio duration via `ffprobe`.
* **Adaptive Chunking**: Files exceeding 1 hour (3600s) or triggering HTTP 413 are automatically normalized to 16kHz mono MP3 via `ffmpeg`, split into 600s chunks, transcribed sequentially, and merged.

### 4. 🤖 Agent-First JSON-Only Contract
Designed from the ground up for LLM coding agents (Claude Code, Cursor, Codex, Pi) and shell pipelines:
* **stdout is 100% JSON**: Success and error payloads both emit standard JSON objects.
* **stderr for Diagnostics**: Human-facing progress, prompts, and debug logs are routed strictly to stderr.
* **Inline vs. Preview Policy**: Automatically inlines full transcript for short audio (`inline_mode: "full"`), and yields line previews plus file path for long audio (`inline_mode: "preview"`) to prevent agent context overflow.

### 5. 🔑 Unified Multi-Platform Authentication (`auth <adapter>`)
* Secure native credential storage: macOS **Keychain**, Windows **Credential Manager**, and Linux configuration file (`~/.memos2txt/config.json`).
* Seamless fallback to environment variables (`GROQ_API_KEY`, `DEEPGRAM_API_KEY`, `ASSEMBLYAI_API_KEY`).

---

## ⚖️ Why memos2txt?

| Feature | Local Whisper (`whisper.cpp` / MLX) | Python Scripts (`openai-whisper`) | Cloud Web Consoles | memos2txt |
| --- | --- | --- | --- | --- |
| **System Overhead** | Heavy GPU/RAM usage, slow on CPU | Huge Python / PyTorch dependencies | Manual drag-and-drop | ✅ **Lightweight (~6MB static binary)** |
| **Agent Friendly** | Mixed stdout logs and stderr noise | Unstable stdout formatting | No terminal / CLI interface | ✅ **Deterministic stdout JSON only** |
| **Deduplication** | Manual caching logic needed | Re-transcribes every run | No automated caching | ✅ **Automatic SHA256 content cache** |
| **Large Audio Handling** | Out-of-memory crashes on long audio | Complex chunking boilerplate | Hard file size ceilings | ✅ **Automated probe & ffmpeg chunking** |
| **Multi-Provider** | Locked to Whisper weights | Separate SDKs for each vendor | Fragmented web consoles | ✅ **Groq + Deepgram + AssemblyAI unified** |

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
# 1. Store your API key into native Keychain / Credential Manager
echo "gsk_your_groq_key" | memos2txt auth groq --api-key-stdin
echo "your_deepgram_key" | memos2txt auth deepgram --api-key-stdin

# 2. Check configuration status
memos2txt auth --list

# 3. Transcribe an Audio File
memos2txt --provider groq --file "/path/to/meeting.m4a" --json
memos2txt --provider deepgram --file "/path/to/interview.m4a" --json
```

---

## 🛠️ Command Reference

### Authentication Commands

```bash
# Interactive setup (masked password entry)
memos2txt auth groq
memos2txt auth deepgram
memos2txt auth assemblyai

# Non-interactive / CI setup
echo "$GROQ_API_KEY" | memos2txt auth groq --api-key-stdin

# List authentication status across all adapters
memos2txt auth --list

# Delete stored credential
memos2txt auth groq --unset
```

### Transcription Flags

```bash
memos2txt --provider <groq|deepgram|assemblyai> --file <path> [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--file <path>` | *(required)* | Path to local audio file (`.m4a`, `.caf`, `.mp3`, `.wav`). |
| `--provider <name>` | *(required)* | Cloud provider: `groq`, `deepgram`, or `assemblyai`. |
| `--model <string>` | Provider default | Model override (`whisper-large-v3`, `nova-3`, `best`). |
| `--language <string>` | `auto` | Language hint (`en`, `zh`, `es`, etc.). |
| `--diarization` | `true` *(Deepgram)* | Enable speaker diarization and speaker labeling. |
| `--punctuate` | `true` | Enable smart punctuation and formatting. |
| `--no-cache` | `false` | Force re-transcription by bypassing disk cache. |
| `--inline-max-chars N` | `4000` | Inline full text if chars <= N; otherwise return preview. |
| `--preview-lines N` | `5` | Number of preview lines returned when transcript is long. |
| `--auto-preprocess` | `true` | Normalize audio to 16kHz mono MP3 via ffmpeg if >25MB. |
| `--chunk-seconds N` | `0` *(auto)* | `0` = auto-probe duration (chunks if >1h); `>0` forces chunk slice. |
| `--timeout <sec>` | `300` | HTTP request timeout in seconds. |

---

## 📋 Output Contract

### Success JSON Response (Short Transcript)

```json
{
  "ok": true,
  "cache_hit": false,
  "provider": "groq",
  "model": "whisper-large-v3",
  "mode": "transcribe",
  "options": {
    "language": "auto",
    "punctuate": true
  },
  "input": {
    "file_path": "/path/to/audio.m4a",
    "file_size_bytes": 524288,
    "mtime_unix": 1700000000
  },
  "output": {
    "transcript_path": "/Users/user/.memos2txt/transcripts/abc123hash.groq.whisper-large-v3.auto.d0.p1.txt",
    "inline_mode": "full",
    "transcript_full": "Good morning. Here is the full transcription of our discussion.",
    "chars": 62,
    "lines": 1
  },
  "version": "0.1.0"
}
```

### Success JSON Response (Long Transcript with Preview)

```json
{
  "ok": true,
  "cache_hit": true,
  "provider": "deepgram",
  "model": "nova-3",
  "output": {
    "transcript_path": "/Users/user/.memos2txt/transcripts/xyz987hash.deepgram.nova-3.auto.d1.p1.txt",
    "inline_mode": "preview",
    "transcript_preview": [
      "Speaker 0: Welcome to the quarterly financial review.",
      "Speaker 1: Thank you. Let's start with revenue numbers..."
    ],
    "chars": 52000,
    "lines": 420
  },
  "note": "Transcript too long; read transcript_path for full text.",
  "version": "0.1.0"
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

`memos2txt` is written in pure Go (`CGO_ENABLED=0`) and natively supports:
* **macOS** (`darwin/arm64`, `darwin/amd64`): Native Apple Keychain integration.
* **Windows** (`windows/amd64`, `windows/arm64`): Native Windows Credential Manager integration.
* **Linux** (`linux/amd64`, `linux/arm64`): Config store at `~/.memos2txt/config.json`.

---

## 📄 License

MIT
