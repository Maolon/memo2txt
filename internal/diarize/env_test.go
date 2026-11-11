package diarize

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnvStatusForReadsMarker(t *testing.T) {
	cacheDir := t.TempDir()
	venv := filepath.Join(cacheDir, "envs", EnvKind)
	if err := os.MkdirAll(venv, 0o700); err != nil {
		t.Fatal(err)
	}
	python := pythonExecutable(venv)
	if err := os.MkdirAll(filepath.Dir(python), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(python, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	marker := envMarker{
		Version:         1,
		Kind:            EnvKind,
		Python:          python,
		PythonVersion:   "3.12.1",
		PyannoteVersion: "4.0.7",
		CreatedUnix:     1,
	}
	b, err := json.Marshal(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(venv, "memos2txt-env.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}

	st := EnvStatusFor(cacheDir)
	if !st.Installed {
		t.Fatal("expected installed=true")
	}
	if st.PythonVersion != "3.12.1" || st.PyannoteVersion != "4.0.7" {
		t.Fatalf("unexpected versions: %+v", st)
	}
	if runtime.GOOS != "windows" && st.Python != python {
		t.Fatalf("python = %q, want %q", st.Python, python)
	}
}

func TestEnvStatusForMissingMarker(t *testing.T) {
	st := EnvStatusFor(t.TempDir())
	if st.Installed {
		t.Fatal("expected installed=false without marker")
	}
}
