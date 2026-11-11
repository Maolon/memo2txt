package providers

import "strings"

// Category groups transcription providers by where inference runs.
type Category string

const (
	CategoryCloud Category = "cloud"
	CategoryLocal Category = "local"
)

// Spec is the static description of a transcription provider.
type Spec struct {
	ID                   string
	Category             Category
	EnvVar               string
	DefaultModel         string
	NativeDiarization    bool
	RequiresAppleSilicon bool
}

var specs = []Spec{
	{
		ID:           "groq",
		Category:     CategoryCloud,
		EnvVar:       "GROQ_API_KEY",
		DefaultModel: "whisper-large-v3",
	},
	{
		ID:                "deepgram",
		Category:          CategoryCloud,
		EnvVar:            "DEEPGRAM_API_KEY",
		DefaultModel:      "nova-3",
		NativeDiarization: true,
	},
	{
		ID:           "assemblyai",
		Category:     CategoryCloud,
		EnvVar:       "ASSEMBLYAI_API_KEY",
		DefaultModel: "best",
	},
	{
		ID:                   "mlx-whisper",
		Category:             CategoryLocal,
		DefaultModel:         "mlx-community/whisper-large-v3-turbo",
		RequiresAppleSilicon: true,
	},
}

// AllProviders returns every transcription provider id.
func AllProviders() []string {
	out := make([]string, 0, len(specs))
	for _, s := range specs {
		out = append(out, s.ID)
	}
	return out
}

// AllAuthAdapters returns every adapter whose credential can be stored through
// the auth command. Hugging Face is a model-hub credential, not an ASR provider.
func AllAuthAdapters() []string {
	out := AllProviders()
	return append(out, "huggingface")
}

func SpecFor(provider string) (Spec, bool) {
	id := strings.ToLower(strings.TrimSpace(provider))
	for _, s := range specs {
		if s.ID == id {
			return s, true
		}
	}
	return Spec{}, false
}

func IsLocal(provider string) bool {
	s, ok := SpecFor(provider)
	return ok && s.Category == CategoryLocal
}

func CategoryFor(provider string) Category {
	if s, ok := SpecFor(provider); ok {
		return s.Category
	}
	return ""
}

func DefaultModel(provider string) string {
	if s, ok := SpecFor(provider); ok {
		return s.DefaultModel
	}
	return ""
}

// NormalizeAuthAdapter maps aliases to the canonical adapter id.
func NormalizeAuthAdapter(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "hf", "huggingface", "hugging-face", "pyannote", "huggingface-hub":
		return "huggingface"
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

func EnvVarForProvider(provider string) (string, bool) {
	if NormalizeAuthAdapter(provider) == "huggingface" {
		return "HF_TOKEN", true
	}
	if s, ok := SpecFor(provider); ok && s.EnvVar != "" {
		return s.EnvVar, true
	}
	return "", false
}
