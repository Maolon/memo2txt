package diarize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"memos2txt/internal/schema"
)

const (
	DefaultModel = "pyannote/speaker-diarization-community-1"
	EnvKind      = "pyannote"
)

// InstallOptions controls the managed Python environment used by pyannote.
type InstallOptions struct {
	PythonSpec string
	Force      bool
	Verbose    bool
	Stderr     io.Writer
	CacheDir   string // ~/.memos2txt by default
}

type EnvStatus struct {
	Installed       bool
	VenvPath        string
	Python          string
	PythonVersion   string
	PyannoteVersion string
	Uv              string
	MarkerPath      string
}

type envMarker struct {
	Version         int    `json:"version"`
	Kind            string `json:"kind"`
	Python          string `json:"python"`
	PythonVersion   string `json:"python_version"`
	PyannoteVersion string `json:"pyannote_version"`
	CreatedUnix     int64  `json:"created_unix"`
}

func DefaultCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".memos2txt"), nil
}

func EnvStatusFor(cacheDir string) EnvStatus {
	if cacheDir == "" {
		cacheDir, _ = DefaultCacheDir()
	}
	venv := filepath.Join(cacheDir, "envs", EnvKind)
	python := pythonExecutable(venv)
	markerPath := filepath.Join(venv, "memos2txt-env.json")

	st := EnvStatus{
		VenvPath:   venv,
		Python:     python,
		MarkerPath: markerPath,
		Uv:         findUV(),
	}
	if _, err := os.Stat(python); err != nil {
		return st
	}
	b, err := os.ReadFile(markerPath)
	if err != nil {
		return st
	}
	var m envMarker
	if err := json.Unmarshal(b, &m); err != nil {
		return st
	}
	st.Installed = true
	st.PythonVersion = m.PythonVersion
	st.PyannoteVersion = m.PyannoteVersion
	return st
}

func pythonExecutable(venv string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venv, "Scripts", "python.exe")
	}
	return filepath.Join(venv, "bin", "python")
}

func findUV() string {
	if configured := strings.TrimSpace(os.Getenv("MEMOS2TXT_UV_BIN")); configured != "" {
		return configured
	}
	if p, err := exec.LookPath("uv"); err == nil {
		return p
	}
	return ""
}

// Install creates (or recreates) the managed pyannote environment.
func Install(ctx context.Context, opts InstallOptions) (EnvStatus, error) {
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if strings.TrimSpace(opts.PythonSpec) == "" {
		opts.PythonSpec = "3.12"
	}
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		var err error
		cacheDir, err = DefaultCacheDir()
		if err != nil {
			return EnvStatus{}, err
		}
	}

	envDir := filepath.Join(cacheDir, "envs", EnvKind)
	if opts.Force {
		if err := os.RemoveAll(envDir); err != nil {
			return EnvStatus{}, fmt.Errorf("remove existing environment: %w", err)
		}
	} else if _, err := os.Stat(filepath.Join(envDir, "memos2txt-env.json")); err != nil {
		// A previous install may have been interrupted before writing the
		// marker. Recreate that partial environment instead of failing inside
		// `uv venv`.
		if err := os.RemoveAll(envDir); err != nil {
			return EnvStatus{}, fmt.Errorf("remove partial environment: %w", err)
		}
	}
	if err := os.MkdirAll(envDir, 0o700); err != nil {
		return EnvStatus{}, fmt.Errorf("create environment dir: %w", err)
	}

	uv := findUV()
	if uv != "" {
		if err := runCommand(ctx, opts, uv, "venv", "--python", opts.PythonSpec, envDir); err != nil {
			return EnvStatus{}, fmt.Errorf("uv venv failed: %w", err)
		}
		python := pythonExecutable(envDir)
		if err := runCommand(ctx, opts, uv, "pip", "install", "--python", python,
			"pyannote.audio>=4.0,<5", "soundfile>=0.12.1"); err != nil {
			return EnvStatus{}, fmt.Errorf("uv pip install failed: %w", err)
		}
	} else {
		python, err := findPython(opts.PythonSpec)
		if err != nil {
			return EnvStatus{}, err
		}
		if err := runCommand(ctx, opts, python, "-m", "venv", envDir); err != nil {
			return EnvStatus{}, fmt.Errorf("python -m venv failed: %w", err)
		}
		venvPython := pythonExecutable(envDir)
		if err := runCommand(ctx, opts, venvPython, "-m", "pip", "install", "--upgrade", "pip"); err != nil {
			return EnvStatus{}, fmt.Errorf("pip upgrade failed: %w", err)
		}
		if err := runCommand(ctx, opts, venvPython, "-m", "pip", "install",
			"pyannote.audio>=4.0,<5", "soundfile>=0.12.1"); err != nil {
			return EnvStatus{}, fmt.Errorf("pip install failed: %w", err)
		}
	}

	venvPython := pythonExecutable(envDir)
	versionOut, err := exec.CommandContext(ctx, venvPython, "-c",
		"import pyannote.audio, soundfile, sys; print(sys.version.split()[0]); print(pyannote.audio.__version__)").Output()
	if err != nil {
		detail := err.Error()
		if ee, ok := err.(*exec.ExitError); ok {
			detail = strings.TrimSpace(string(ee.Stderr))
		}
		return EnvStatus{}, fmt.Errorf("verify pyannote environment: %s", detail)
	}
	lines := strings.Split(strings.TrimSpace(string(versionOut)), "\n")
	pythonVersion, pyannoteVersion := "", ""
	if len(lines) > 0 {
		pythonVersion = strings.TrimSpace(lines[0])
	}
	if len(lines) > 1 {
		pyannoteVersion = strings.TrimSpace(lines[1])
	}

	marker := envMarker{
		Version:         1,
		Kind:            EnvKind,
		Python:          venvPython,
		PythonVersion:   pythonVersion,
		PyannoteVersion: pyannoteVersion,
		CreatedUnix:     time.Now().Unix(),
	}
	b, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return EnvStatus{}, err
	}
	if err := os.WriteFile(filepath.Join(envDir, "memos2txt-env.json"), b, 0o600); err != nil {
		return EnvStatus{}, fmt.Errorf("write environment marker: %w", err)
	}

	st := EnvStatusFor(cacheDir)
	return st, nil
}

func findPython(spec string) (string, error) {
	if strings.ContainsRune(spec, os.PathSeparator) {
		if _, err := os.Stat(spec); err == nil {
			return spec, nil
		}
		return "", fmt.Errorf("python interpreter not found: %s", spec)
	}
	candidates := []string{spec, "python" + spec, "python3"}
	if spec == "3" || spec == "" {
		candidates = []string{"python3", "python"}
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("python %s not found; install uv (recommended) or Python 3.12", spec)
}

func runCommand(ctx context.Context, opts InstallOptions, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = opts.Stderr
	cmd.Stderr = opts.Stderr
	if opts.Verbose {
		fmt.Fprintf(opts.Stderr, "+ %s %s\n", name, strings.Join(args, " "))
	}
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// EnsureInstalled returns a JSON-ready error when the diarization runtime is
// missing. It does not import torch, so it stays fast on the hot path.
func EnsureInstalled(cacheDir string) (EnvStatus, error) {
	st := EnvStatusFor(cacheDir)
	if !st.Installed {
		return st, schemaError(schema.ErrLocalDependencyMissing,
			"Local diarization runtime is not installed.",
			"Run: memos2txt diarizer install  (installs pyannote.audio + torch in "+st.VenvPath+")")
	}
	return st, nil
}

type schemaProviderError struct {
	Code    schema.ErrorCode
	Message string
	Detail  string
}

func (e schemaProviderError) Error() string { return e.Message + ": " + e.Detail }

func schemaError(code schema.ErrorCode, message, detail string) error {
	return schemaProviderError{Code: code, Message: message, Detail: detail}
}

// MapError converts a diarize error into the JSON error contract.
func MapError(err error) (schema.ErrorCode, string, string, bool) {
	var pe schemaProviderError
	if errors.As(err, &pe) {
		return pe.Code, pe.Message, pe.Detail, true
	}
	return "", "", "", false
}
