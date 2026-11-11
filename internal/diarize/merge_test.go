package diarize

import (
	"reflect"
	"testing"
)

func TestAssignSpeakersWordLevel(t *testing.T) {
	turns := []Turn{
		{Start: 0, End: 2, Speaker: "SPEAKER_00"},
		{Start: 2.05, End: 4, Speaker: "SPEAKER_01"},
	}
	segments := []Segment{{
		Start: 0,
		End:   3.2,
		Text:  " Hello there General Kenobi",
		Words: []Word{
			{Text: " Hello", Start: 0, End: 1},
			{Text: " there", Start: 1, End: 1.8},
			{Text: " General", Start: 2.1, End: 2.5},
			{Text: " Kenobi", Start: 2.5, End: 3.2},
		},
	}}

	utterances, speakers := AssignSpeakers(segments, turns, MergeOptions{})
	want := []Utterance{
		{Start: 0, End: 1.8, Speaker: "SPEAKER_00", SpeakerIndex: 0, Label: "Speaker 0", Text: "Hello there"},
		{Start: 2.1, End: 3.2, Speaker: "SPEAKER_01", SpeakerIndex: 1, Label: "Speaker 1", Text: "General Kenobi"},
	}
	if !reflect.DeepEqual(utterances, want) {
		t.Fatalf("utterances = %#v, want %#v", utterances, want)
	}
	if len(speakers) != 2 || speakers[0].Label != "Speaker 0" || speakers[1].Label != "Speaker 1" {
		t.Fatalf("speakers = %#v", speakers)
	}
	if got := Render(utterances, RenderOptions{}); got != "[Speaker:0] Hello there\n[Speaker:1] General Kenobi" {
		t.Fatalf("render = %q", got)
	}
}

func TestAssignSpeakersSmoothsShortRuns(t *testing.T) {
	turns := []Turn{
		{Start: 0, End: 1, Speaker: "SPEAKER_00"},
		{Start: 1.0, End: 1.2, Speaker: "SPEAKER_01"},
		{Start: 1.2, End: 2, Speaker: "SPEAKER_00"},
	}
	segments := []Segment{{
		Start: 0,
		End:   1.9,
		Text:  " one two three",
		Words: []Word{
			{Text: " one", Start: 0, End: 0.4},
			{Text: " two", Start: 0.95, End: 1.15},
			{Text: " three", Start: 1.2, End: 1.9},
		},
	}}

	utterances, _ := AssignSpeakers(segments, turns, MergeOptions{})
	if len(utterances) != 1 {
		t.Fatalf("expected one smoothed utterance, got %#v", utterances)
	}
	if utterances[0].Speaker != "SPEAKER_00" || utterances[0].Text != "one two three" {
		t.Fatalf("smoothed utterance = %#v", utterances[0])
	}
}

func TestAssignSpeakersFallbackNearest(t *testing.T) {
	turns := []Turn{{Start: 1, End: 2, Speaker: "SPEAKER_00"}}
	segments := []Segment{{Start: 5, End: 6, Text: "after the turn"}}

	utterances, _ := AssignSpeakers(segments, turns, MergeOptions{})
	if len(utterances) != 1 || utterances[0].Speaker != "SPEAKER_00" {
		t.Fatalf("utterances = %#v", utterances)
	}
}

func TestAssignSpeakersSegmentLevelAndGapMerge(t *testing.T) {
	turns := []Turn{
		{Start: 0, End: 2, Speaker: "A"},
		{Start: 4, End: 6, Speaker: "B"},
	}
	segments := []Segment{
		{Start: 0.2, End: 1.2, Text: "first"},
		{Start: 1.4, End: 1.9, Text: "second"},
		{Start: 4.1, End: 5.0, Text: "third"},
	}

	utterances, _ := AssignSpeakers(segments, turns, MergeOptions{MergeGapSeconds: 1.0})
	want := []Utterance{
		{Start: 0.2, End: 1.9, Speaker: "A", SpeakerIndex: 0, Label: "Speaker 0", Text: "first second"},
		{Start: 4.1, End: 5.0, Speaker: "B", SpeakerIndex: 1, Label: "Speaker 1", Text: "third"},
	}
	if !reflect.DeepEqual(utterances, want) {
		t.Fatalf("utterances = %#v, want %#v", utterances, want)
	}
}

func TestAssignSpeakersCJKJoin(t *testing.T) {
	turns := []Turn{{Start: 0, End: 5, Speaker: "SPEAKER_00"}}
	segments := []Segment{{
		Start: 0,
		End:   2,
		Text:  "你好 世界",
		Words: []Word{
			{Text: "你好", Start: 0, End: 0.8},
			{Text: "世界", Start: 0.9, End: 1.6},
		},
	}}
	utterances, _ := AssignSpeakers(segments, turns, MergeOptions{})
	if len(utterances) != 1 || utterances[0].Text != "你好世界" {
		t.Fatalf("utterances = %#v", utterances)
	}
}

func TestFormatTimestamp(t *testing.T) {
	cases := map[float64]string{
		0:    "00:00",
		65.4: "01:05",
		3661: "01:01:01",
		3599: "59:59",
		-10:  "00:00",
	}
	for in, want := range cases {
		if got := FormatTimestamp(in); got != want {
			t.Errorf("FormatTimestamp(%v) = %q, want %q", in, got, want)
		}
	}
}
