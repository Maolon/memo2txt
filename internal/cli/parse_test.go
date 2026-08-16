package cli

import (
	"testing"
)

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
