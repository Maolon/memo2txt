package output

import (
	"bufio"
	"strings"

	"memos2txt/internal/schema"
)

func Build(transcriptText, transcriptPath string, inlineMaxChars, previewLines int) (schema.OutputInfo, string) {
	if inlineMaxChars <= 0 {
		inlineMaxChars = 4000
	}
	if previewLines <= 0 {
		previewLines = 5
	}

	chars := len([]rune(transcriptText))
	lines, preview := countLinesAndPreview(transcriptText, previewLines)

	out := schema.OutputInfo{
		TranscriptPath: transcriptPath,
		Chars:          chars,
		Lines:          lines,
	}

	if chars <= inlineMaxChars {
		out.InlineMode = "full"
		out.TranscriptFull = strings.TrimSuffix(transcriptText, "\n")
		out.TranscriptPreview = []string{}
		return out, ""
	}

	out.InlineMode = "preview"
	out.TranscriptFull = ""
	out.TranscriptPreview = preview
	return out, "Transcript too long; read transcript_path for full text."
}

func countLinesAndPreview(s string, previewLines int) (lines int, preview []string) {
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		lines++
		if len(preview) < previewLines {
			preview = append(preview, sc.Text())
		}
	}
	return lines, preview
}
