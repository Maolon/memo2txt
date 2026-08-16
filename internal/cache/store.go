package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type TranscriptStore struct {
	Dir string
}

func NewTranscriptStore(dir string) TranscriptStore {
	return TranscriptStore{Dir: dir}
}

type Key struct {
	SHA256      string
	Provider    string
	Model       string
	Language    string
	Diarization bool
	Punctuate   bool
}

type Metadata struct {
	Version        int    `json:"version"`
	Key            Key    `json:"key"`
	InputFilePath  string `json:"input_file_path"`
	InputSizeBytes int64  `json:"input_size_bytes"`
	InputMtimeUnix int64  `json:"input_mtime_unix"`
	CreatedUnix    int64  `json:"created_unix"`
}

func (s TranscriptStore) HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s TranscriptStore) TranscriptPath(k Key) string {
	return filepath.Join(s.Dir, safeFileName(k)+".txt")
}

func (s TranscriptStore) MetadataPath(k Key) string {
	return filepath.Join(s.Dir, safeFileName(k)+".json")
}

func (s TranscriptStore) ReadIfExists(path string) (text string, ok bool, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	if len(b) == 0 {
		return "", false, nil
	}
	return string(b), true, nil
}

func (s TranscriptStore) WriteTranscript(path, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s TranscriptStore) WriteMetadata(path string, md Metadata) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func safeFileName(k Key) string {
	model := strings.ReplaceAll(k.Model, ".", "_")
	lang := k.Language
	if strings.TrimSpace(lang) == "" {
		lang = "auto"
	}
	lang = strings.ReplaceAll(lang, ".", "_")

	return strings.Join([]string{
		k.SHA256,
		k.Provider,
		model,
		lang,
		boolToken(k.Diarization, "d1", "d0"),
		boolToken(k.Punctuate, "p1", "p0"),
	}, ".")
}

func boolToken(v bool, t, f string) string {
	if v {
		return t
	}
	return f
}
