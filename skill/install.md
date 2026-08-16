---
name: memo2txt-install
description: "Installation reference for the memo2txt skill and CLI. Loaded by the parent SKILL.md and by the `npx skills add` flow."
---

# Installing memo2txt

This skill shells out to the `memos2txt` CLI. The CLI binary must be installed and authenticated before any command in `SKILL.md` will work.

## Quick Install

### Option 1: One-Line Curl Installer (macOS & Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/Maolon/memo2txt/main/install.sh | bash
```

### Option 2: Homebrew (macOS & Linux)

```bash
brew install Maolon/tap/memos2txt
```

### Option 3: Prebuilt Release Binaries

Download from the latest release: <https://github.com/Maolon/memo2txt/releases/latest>

Extract and copy `memos2txt` into your PATH (e.g. `/usr/local/bin/` or `~/.local/bin/`).

---

## Agent Verification & Setup

After installing the binary, configure at least one cloud ASR API key:

```bash
# Check configuration
memos2txt auth --list

# Set Groq or Deepgram key
echo "$GROQ_API_KEY" | memos2txt auth groq --api-key-stdin
echo "$DEEPGRAM_API_KEY" | memos2txt auth deepgram --api-key-stdin
```

Verify ready state:
```bash
memos2txt -h
```
