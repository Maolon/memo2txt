package cli

import (
	"testing"
)

func TestParseLocalProviderFlags(t *testing.T) {
	cfg, res := Parse([]string{
		"--provider", "mlx-whisper",
		"--file", "/tmp/a.wav",
		"--diarizer", "pyannote",
		"--diarizer-device", "cpu",
		"--num-speakers", "2",
		"--timestamps",
		"--whisper-prompt", "ORRBSE",
	})
	if res != ParseResultOK {
		t.Fatalf("result = %v (%s)", res, cfg.ParseError)
	}
	if cfg.Provider != "mlx-whisper" || cfg.Diarizer != "pyannote" || cfg.DiarizerDevice != "cpu" {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	if cfg.NumSpeakers != 2 || !cfg.Timestamps || cfg.WhisperPrompt != "ORRBSE" {
		t.Fatalf("unexpected local flags: %+v", cfg)
	}
	if cfg.TimeoutSeconds != 3600 || cfg.TimeoutSet {
		t.Fatalf("local default timeout = %d set=%v, want 3600 unset", cfg.TimeoutSeconds, cfg.TimeoutSet)
	}
}

func TestParseExplicitTimeoutWins(t *testing.T) {
	cfg, res := Parse([]string{"--provider", "mlx-whisper", "--file", "/tmp/a.wav", "--timeout", "42"})
	if res != ParseResultOK {
		t.Fatalf("result = %v", res)
	}
	if cfg.TimeoutSeconds != 42 || !cfg.TimeoutSet {
		t.Fatalf("timeout = %d set=%v", cfg.TimeoutSeconds, cfg.TimeoutSet)
	}
}

func TestParseDiarizerCommand(t *testing.T) {
	tests := []struct {
		args       []string
		wantAction string
		wantPython string
		wantForce  bool
	}{
		{args: []string{"diarizer", "install"}, wantAction: "install", wantPython: "3.12"},
		{args: []string{"diarizer", "install", "--python", "3.11", "--force"}, wantAction: "install", wantPython: "3.11", wantForce: true},
		{args: []string{"diarizer"}, wantAction: "status", wantPython: ""},
		{args: []string{"diarizer", "uninstall"}, wantAction: "uninstall", wantPython: ""},
	}
	for _, tt := range tests {
		cfg, res := Parse(tt.args)
		if res != ParseResultOK {
			t.Fatalf("%v: result = %v (%s)", tt.args, res, cfg.ParseError)
		}
		if !cfg.DiarizerCommand {
			t.Fatalf("%v: DiarizerCommand=false", tt.args)
		}
		if cfg.DiarizerAction != tt.wantAction {
			t.Errorf("%v: action = %q, want %q", tt.args, cfg.DiarizerAction, tt.wantAction)
		}
		if tt.wantPython != "" && cfg.DiarizerPython != tt.wantPython {
			t.Errorf("%v: python = %q, want %q", tt.args, cfg.DiarizerPython, tt.wantPython)
		}
		if cfg.DiarizerForce != tt.wantForce {
			t.Errorf("%v: force = %v, want %v", tt.args, cfg.DiarizerForce, tt.wantForce)
		}
	}
}

func TestParseAuthCommand(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantProvider string
		wantAuthMode bool
		wantUnset    bool
		wantList     bool
		wantStdin    bool
		wantResult   ParseResult
	}{
		{
			name:         "auth groq",
			args:         []string{"auth", "groq"},
			wantProvider: "groq",
			wantAuthMode: true,
			wantResult:   ParseResultOK,
		},
		{
			name:         "auth deepgram with stdin",
			args:         []string{"auth", "deepgram", "--api-key-stdin"},
			wantProvider: "deepgram",
			wantAuthMode: true,
			wantStdin:    true,
			wantResult:   ParseResultOK,
		},
		{
			name:         "auth assemblyai unset",
			args:         []string{"auth", "assemblyai", "--unset"},
			wantProvider: "assemblyai",
			wantAuthMode: true,
			wantUnset:    true,
			wantResult:   ParseResultOK,
		},
		{
			name:         "auth list",
			args:         []string{"auth", "--list"},
			wantAuthMode: true,
			wantList:     true,
			wantResult:   ParseResultOK,
		},
		{
			name:         "auth without args defaults to auth mode",
			args:         []string{"auth"},
			wantAuthMode: true,
			wantResult:   ParseResultOK,
		},
		{
			name:         "legacy setup flag",
			args:         []string{"--setup", "--provider", "groq"},
			wantProvider: "groq",
			wantResult:   ParseResultOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, res := Parse(tt.args)
			if res != tt.wantResult {
				t.Fatalf("Parse() result = %v, want %v", res, tt.wantResult)
			}
			if tt.wantProvider != "" && cfg.Provider != tt.wantProvider {
				t.Errorf("cfg.Provider = %q, want %q", cfg.Provider, tt.wantProvider)
			}
			if tt.wantAuthMode && !cfg.AuthMode {
				t.Errorf("cfg.AuthMode = %v, want true", cfg.AuthMode)
			}
			if tt.wantUnset && !cfg.UnsetAPIKey {
				t.Errorf("cfg.UnsetAPIKey = %v, want true", cfg.UnsetAPIKey)
			}
			if tt.wantList && !cfg.AuthList {
				t.Errorf("cfg.AuthList = %v, want true", cfg.AuthList)
			}
			if tt.wantStdin && !cfg.APIKeyStdin {
				t.Errorf("cfg.APIKeyStdin = %v, want true", cfg.APIKeyStdin)
			}
		})
	}
}
