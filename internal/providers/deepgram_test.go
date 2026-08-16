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

func TestDeepgramTranscribeSetsTokenAndQuery(t *testing.T) {
	var gotAuth string
	var gotCT string
	var gotQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		gotQuery = r.URL.RawQuery

		_ = r.Body.Close()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": map[string]any{
				"utterances": []any{
					map[string]any{
						"start":      0.0,
						"end":        1.0,
						"speaker":    0,
						"transcript": "hello",
						"confidence": 0.9,
					},
					map[string]any{
						"start":      1.0,
						"end":        2.0,
						"speaker":    1,
						"transcript": "world",
						"confidence": 0.9,
					},
				},
				"channels": []any{
					map[string]any{
						"alternatives": []any{
							map[string]any{
								"transcript": "hello deepgram",
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	audio := writeTempFile(t, "a.m4a", "fake-audio")
	c := NewDeepgramClient("k", DeepgramClientOptions{BaseURL: srv.URL, Timeout: 2 * time.Second, Model: "nova-3"})
	text, err := c.Transcribe(context.Background(), audio, TranscribeOptions{Language: "en", Punctuate: true, Diarization: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "[Speaker:0] hello") || !strings.Contains(text, "[Speaker:1] world") {
		t.Fatalf("text=%q", text)
	}
	if gotAuth != "Token k" {
		t.Fatalf("auth=%q", gotAuth)
	}
	if gotCT != "audio/mp4" {
		t.Fatalf("content-type=%q", gotCT)
	}
	if !strings.Contains(gotQuery, "model=nova-3") ||
		!strings.Contains(gotQuery, "punctuate=true") ||
		!strings.Contains(gotQuery, "language=en") ||
		!strings.Contains(gotQuery, "diarize=true") ||
		!strings.Contains(gotQuery, "utterances=true") ||
		!strings.Contains(gotQuery, "smart_format=true") {
		t.Fatalf("query=%q", gotQuery)
	}
}
