package diarize

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Segment is a timed ASR fragment. Words are optional; when present they are
// used for word-level speaker assignment.
type Segment struct {
	Start float64
	End   float64
	Text  string
	Words []Word
}

type Word struct {
	Text  string
	Start float64
	End   float64
}

// Turn is one speaker segment produced by a diarizer.
type Turn struct {
	Start   float64
	End     float64
	Speaker string
}

// Utterance is a merged, speaker-labeled transcript line.
type Utterance struct {
	Start        float64
	End          float64
	Speaker      string // canonical id, e.g. SPEAKER_00
	SpeakerIndex int    // 0-based index used in [Speaker:N]
	Label        string // human label, e.g. Speaker 0
	Text         string
}

type SpeakerSummary struct {
	ID            string
	Index         int
	Label         string
	Turns         int
	SpeechSeconds float64
}

type MergeOptions struct {
	// MergeGapSeconds joins consecutive utterances by the same speaker when
	// the silence gap is at most this long. Default 1.5s.
	MergeGapSeconds float64
	// MinRunSeconds smooths word-level speaker runs shorter than this when
	// both neighbours belong to the same speaker. Default 0.6s.
	MinRunSeconds float64
}

type RenderOptions struct {
	Timestamps bool
}

// AssignSpeakers maps ASR segments to diarization turns and renders utterances.
func AssignSpeakers(segments []Segment, turns []Turn, opts MergeOptions) ([]Utterance, []SpeakerSummary) {
	if opts.MergeGapSeconds <= 0 {
		opts.MergeGapSeconds = 1.5
	}
	if opts.MinRunSeconds <= 0 {
		opts.MinRunSeconds = 0.6
	}

	normalized := normalizeTurns(turns)
	if len(normalized) == 0 {
		return nil, nil
	}

	labelByID := make(map[string]string, len(normalized))
	indexByID := make(map[string]int, len(normalized))
	order := make([]string, 0, len(normalized))
	for _, t := range normalized {
		if _, ok := labelByID[t.Speaker]; !ok {
			idx := len(order)
			order = append(order, t.Speaker)
			indexByID[t.Speaker] = idx
			labelByID[t.Speaker] = fmt.Sprintf("Speaker %d", idx)
		}
	}

	units := buildUnits(segments)
	if len(units) == 0 {
		return nil, nil
	}

	prev := ""
	for i := range units {
		units[i].speaker = bestSpeaker(units[i].start, units[i].end, normalized)
		if units[i].speaker == "" {
			units[i].speaker = prev
		}
		if units[i].speaker == "" {
			units[i].speaker = order[0]
		}
		prev = units[i].speaker
	}

	if unitsUseWords(segments) && len(units) > 1 {
		smoothRuns(units, opts.MinRunSeconds)
	}

	utterances := groupUnits(units, opts.MergeGapSeconds)
	for i := range utterances {
		utterances[i].SpeakerIndex = indexByID[utterances[i].Speaker]
		utterances[i].Label = labelByID[utterances[i].Speaker]
		if utterances[i].Label == "" {
			utterances[i].Label = "Speaker 0"
		}
	}

	summaries := summarize(utterances, order, labelByID, indexByID)
	return utterances, summaries
}

type unit struct {
	start    float64
	end      float64
	text     string
	speaker  string
	fromWord bool
}

func buildUnits(segments []Segment) []unit {
	var units []unit
	for _, seg := range segments {
		if len(seg.Words) > 0 {
			for _, w := range seg.Words {
				text := strings.TrimSpace(w.Text)
				if text == "" {
					continue
				}
				units = append(units, unit{start: w.Start, end: w.End, text: text, fromWord: true})
			}
			continue
		}
		text := strings.TrimSpace(seg.Text)
		if text == "" {
			continue
		}
		units = append(units, unit{start: seg.Start, end: seg.End, text: text})
	}
	return units
}

func unitsUseWords(segments []Segment) bool {
	words, plain := 0, 0
	for _, s := range segments {
		if len(s.Words) > 0 {
			words++
		} else if strings.TrimSpace(s.Text) != "" {
			plain++
		}
	}
	return words > 0 && plain == 0
}

func normalizeTurns(turns []Turn) []Turn {
	out := make([]Turn, 0, len(turns))
	for _, t := range turns {
		if t.End <= t.Start {
			continue
		}
		speaker := strings.TrimSpace(t.Speaker)
		if speaker == "" {
			continue
		}
		out = append(out, Turn{Start: t.Start, End: t.End, Speaker: speaker})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Start == out[j].Start {
			return out[i].End < out[j].End
		}
		return out[i].Start < out[j].Start
	})
	return out
}

func bestSpeaker(start, end float64, turns []Turn) string {
	best := ""
	bestOverlap := 0.0
	for _, t := range turns {
		o := overlap(start, end, t.Start, t.End)
		if o > bestOverlap {
			bestOverlap = o
			best = t.Speaker
		}
	}
	if bestOverlap > 0 {
		return best
	}

	// No overlap (silence, padding, or hallucinated text): use the nearest turn.
	bestDistance := math.MaxFloat64
	for _, t := range turns {
		d := intervalDistance(start, end, t.Start, t.End)
		if d < bestDistance {
			bestDistance = d
			best = t.Speaker
		}
	}
	return best
}

func overlap(aStart, aEnd, bStart, bEnd float64) float64 {
	start := math.Max(aStart, bStart)
	end := math.Min(aEnd, bEnd)
	if end <= start {
		return 0
	}
	return end - start
}

func intervalDistance(aStart, aEnd, bStart, bEnd float64) float64 {
	if aEnd < bStart {
		return bStart - aEnd
	}
	if bEnd < aStart {
		return aStart - bEnd
	}
	return 0
}

func smoothRuns(units []unit, minRunSeconds float64) {
	i := 0
	for i < len(units) {
		j := i + 1
		for j < len(units) && units[j].speaker == units[i].speaker {
			j++
		}
		if i > 0 && j < len(units) {
			runDuration := units[j-1].end - units[i].start
			if runDuration < minRunSeconds && units[i-1].speaker == units[j].speaker {
				for k := i; k < j; k++ {
					units[k].speaker = units[i-1].speaker
				}
			}
		}
		i = j
	}
}

func groupUnits(units []unit, mergeGap float64) []Utterance {
	var out []Utterance
	for _, u := range units {
		n := len(out)
		if n > 0 {
			last := &out[n-1]
			gap := u.start - last.End
			if last.Speaker == u.speaker && gap <= mergeGap {
				last.Text = joinText(last.Text, u.text)
				if u.end > last.End {
					last.End = u.end
				}
				continue
			}
		}
		out = append(out, Utterance{
			Start:   u.start,
			End:     u.end,
			Speaker: u.speaker,
			Text:    strings.TrimSpace(u.text),
		})
	}
	return out
}

func summarize(utterances []Utterance, order []string, labels map[string]string, indexes map[string]int) []SpeakerSummary {
	byID := make(map[string]*SpeakerSummary, len(order))
	for _, id := range order {
		byID[id] = &SpeakerSummary{ID: id, Index: indexes[id], Label: labels[id]}
	}
	for _, u := range utterances {
		s, ok := byID[u.Speaker]
		if !ok {
			s = &SpeakerSummary{ID: u.Speaker, Index: u.SpeakerIndex, Label: u.Label}
			byID[u.Speaker] = s
		}
		s.Turns++
		s.SpeechSeconds += math.Max(0, u.End-u.Start)
	}
	out := make([]SpeakerSummary, 0, len(order))
	for _, id := range order {
		s := byID[id]
		if s == nil || s.Turns == 0 {
			continue
		}
		s.SpeechSeconds = math.Round(s.SpeechSeconds*100) / 100
		out = append(out, *s)
	}
	return out
}

func joinText(prev, next string) string {
	prev = strings.TrimSpace(prev)
	next = strings.TrimSpace(next)
	if prev == "" {
		return next
	}
	if next == "" {
		return prev
	}
	last, first := lastRune(prev), firstRune(next)
	if isCJK(last) && isCJK(first) {
		return prev + next
	}
	return prev + " " + next
}

func lastRune(s string) rune {
	r := []rune(s)
	if len(r) == 0 {
		return 0
	}
	return r[len(r)-1]
}

func firstRune(s string) rune {
	r := []rune(s)
	if len(r) == 0 {
		return 0
	}
	return r[0]
}

func isCJK(r rune) bool {
	switch {
	case r >= 0x3040 && r <= 0x30FF: // Hiragana / Katakana
		return true
	case r >= 0x3400 && r <= 0x4DBF: // CJK Extension A
		return true
	case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
		return true
	case r >= 0xF900 && r <= 0xFAFF: // CJK Compatibility Ideographs
		return true
	case r >= 0xAC00 && r <= 0xD7AF: // Hangul
		return true
	}
	return false
}

// Render formats utterances as transcript lines.
func Render(utterances []Utterance, opts RenderOptions) string {
	var b strings.Builder
	for _, u := range utterances {
		text := strings.TrimSpace(u.Text)
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		if opts.Timestamps {
			b.WriteString("[")
			b.WriteString(FormatTimestamp(u.Start))
			b.WriteString("] ")
		}
		fmt.Fprintf(&b, "[Speaker:%d] %s", u.SpeakerIndex, text)
	}
	return strings.TrimSpace(b.String())
}

// FormatTimestamp renders seconds as MM:SS, or HH:MM:SS for long recordings.
func FormatTimestamp(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	total := int(math.Round(seconds))
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}
