package providers

import (
	"encoding/json"
	"testing"
)

func TestSegmentsFromMLX(t *testing.T) {
	raw := []byte(`{
	  "text": " Hello world. Second line.",
	  "language": "en",
	  "segments": [
	    {"start": 0.0, "end": 1.5, "text": " Hello world.", "words": [
	      {"word": " Hello", "start": 0.0, "end": 0.5},
	      {"word": " world.", "start": 0.5, "end": 1.2}
	    ]},
	    {"start": 2.0, "end": 3.0, "text": "  ", "words": []},
	    {"start": 2.1, "end": 3.4, "text": " Second line.", "words": []}
	  ]
	}`)
	var parsed mlxWhisperOutput
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	segments, err := segmentsFromMLX(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 2 {
		t.Fatalf("segments = %#v", segments)
	}
	if segments[0].Text != "Hello world." || len(segments[0].Words) != 2 {
		t.Fatalf("first segment = %#v", segments[0])
	}
	if got := FormatPlainSegments(segments); got != "Hello world.\nSecond line." {
		t.Fatalf("plain = %q", got)
	}
}

func TestSegmentsFromMLXFallbackText(t *testing.T) {
	parsed := mlxWhisperOutput{Text: " fallback text "}
	segments, err := segmentsFromMLX(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 1 || segments[0].Text != "fallback text" {
		t.Fatalf("segments = %#v", segments)
	}
}

func TestSegmentsFromMLXRejectsEmpty(t *testing.T) {
	if _, err := segmentsFromMLX(mlxWhisperOutput{}); err == nil {
		t.Fatal("expected error for empty transcript")
	}
}

func TestProviderRegistry(t *testing.T) {
	if !IsLocal("mlx-whisper") {
		t.Fatal("mlx-whisper must be local")
	}
	if IsLocal("groq") {
		t.Fatal("groq must not be local")
	}
	if got := CategoryFor("mlx-whisper"); got != CategoryLocal {
		t.Fatalf("category = %q", got)
	}
	if got := DefaultModel("mlx-whisper"); got != DefaultMLXWhisperModel {
		t.Fatalf("default model = %q", got)
	}
	if env, ok := EnvVarForProvider("hf"); !ok || env != "HF_TOKEN" {
		t.Fatalf("hf env = %q ok=%v", env, ok)
	}
	if got := NormalizeAuthAdapter("pyannote"); got != "huggingface" {
		t.Fatalf("normalize = %q", got)
	}
}
