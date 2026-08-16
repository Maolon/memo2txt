package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAssemblyAITranscribeUploadCreatePoll(t *testing.T) {
	var gotAuthUpload string
	var gotAuthCreate string
	var gotAuthPoll string
	var gotSpeechModel string
	var gotLanguage string
	var gotPunctuate *bool
	var gotSpeakerLabels *bool

	polls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/upload":
			gotAuthUpload = r.Header.Get("Authorization")
			_ = r.Body.Close()
			_ = json.NewEncoder(w).Encode(map[string]string{"upload_url": "https://upload.example/audio"})
		case r.Method == http.MethodPost && r.URL.Path == "/v2/transcript":
			gotAuthCreate = r.Header.Get("Authorization")
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			_ = r.Body.Close()
			if v, ok := body["speech_model"].(string); ok {
				gotSpeechModel = v
			}
			if v, ok := body["language_code"].(string); ok {
				gotLanguage = v
			}
			if v, ok := body["punctuate"].(bool); ok {
				gotPunctuate = &v
			}
			if v, ok := body["speaker_labels"].(bool); ok {
				gotSpeakerLabels = &v
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "t1"})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v2/transcript/"):
			gotAuthPoll = r.Header.Get("Authorization")
			polls++
			if polls < 2 {
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "t1", "status": "processing"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "t1", "status": "completed", "text": "hello assembly"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	audio := writeTempFile(t, "a.m4a", "fake-audio")
	c := NewAssemblyAIClient("k", AssemblyAIClientOptions{
		BaseURL:      srv.URL,
		Timeout:      2 * time.Second,
		Model:        "best",
		PollInterval: 10 * time.Millisecond,
	})

	text, err := c.Transcribe(context.Background(), audio, TranscribeOptions{Language: "en", Punctuate: true, Diarization: true})
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello assembly" {
		t.Fatalf("text=%q", text)
	}
	if gotAuthUpload != "k" || gotAuthCreate != "k" || gotAuthPoll != "k" {
		t.Fatalf("auth upload/create/poll = %q/%q/%q", gotAuthUpload, gotAuthCreate, gotAuthPoll)
	}
	if gotSpeechModel != "best" {
		t.Fatalf("speech_model=%q", gotSpeechModel)
	}
	if gotLanguage != "en" {
		t.Fatalf("language_code=%q", gotLanguage)
	}
	if gotPunctuate == nil || *gotPunctuate != true {
		t.Fatalf("punctuate=%v", gotPunctuate)
	}
	if gotSpeakerLabels == nil || *gotSpeakerLabels != true {
		t.Fatalf("speaker_labels=%v", gotSpeakerLabels)
	}
	if polls < 2 {
		t.Fatalf("polls=%d", polls)
	}
}
