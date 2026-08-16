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

func TestGroqTranscribeBuildsMultipartRequest(t *testing.T) {
	var gotAuth string
	var gotModel string
	var gotLanguage string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")

		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Fatalf("content-type=%q", r.Header.Get("Content-Type"))
		}

		mr, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("multipart reader: %v", err)
		}
		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}
			b := readAll(t, p)
			switch p.FormName() {
			case "model":
				gotModel = strings.TrimSpace(string(b))
			case "language":
				gotLanguage = strings.TrimSpace(string(b))
			}
		}

		_ = json.NewEncoder(w).Encode(map[string]string{"text": "hello"})
	}))
	defer srv.Close()

	audio := writeTempFile(t, "a.m4a", "fake-audio")
	c := NewGroqClient("k", GroqClientOptions{BaseURL: srv.URL, Timeout: 2 * time.Second, Model: "whisper-large-v3"})
	text, err := c.Transcribe(context.Background(), audio, TranscribeOptions{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello" {
		t.Fatalf("text=%q", text)
	}
	if gotAuth != "Bearer k" {
		t.Fatalf("auth=%q", gotAuth)
	}
	if gotModel != "whisper-large-v3" {
		t.Fatalf("model=%q", gotModel)
	}
	if gotLanguage != "en" {
		t.Fatalf("language=%q", gotLanguage)
	}
}
