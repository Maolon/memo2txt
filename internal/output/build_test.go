package output

import "testing"

func TestBuildInlineFull(t *testing.T) {
	out, note := Build("hello\nworld\n", "/tmp/t.txt", 100, 2)
	if out.InlineMode != "full" {
		t.Fatalf("inline_mode=%q", out.InlineMode)
	}
	if out.TranscriptFull != "hello\nworld" {
		t.Fatalf("transcript_full=%q", out.TranscriptFull)
	}
	if len(out.TranscriptPreview) != 0 {
		t.Fatalf("preview=%v", out.TranscriptPreview)
	}
	if note != "" {
		t.Fatalf("note=%q", note)
	}
	if out.Lines != 2 {
		t.Fatalf("lines=%d", out.Lines)
	}
}

func TestBuildPreview(t *testing.T) {
	out, note := Build("a\nb\nc\nd\ne\n", "/tmp/t.txt", 1, 3)
	if out.InlineMode != "preview" {
		t.Fatalf("inline_mode=%q", out.InlineMode)
	}
	if out.TranscriptFull != "" {
		t.Fatalf("transcript_full=%q", out.TranscriptFull)
	}
	if len(out.TranscriptPreview) != 3 {
		t.Fatalf("preview=%v", out.TranscriptPreview)
	}
	if note == "" {
		t.Fatalf("expected note")
	}
}
