package secrets

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type fileStore struct {
	service string
	path    string
}

type fileConfig struct {
	Version int               `json:"version"`
	Keys    map[string]string `json:"keys"`
}

func newFileStore(service string) (Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &fileStore{
		service: service,
		path:    filepath.Join(home, ".memos2txt", "config.json"),
	}, nil
}

func (s *fileStore) Kind() string { return "file" }

func (s *fileStore) Get(key string) (string, error) {
	cfg, err := s.read()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(cfg.Keys[key]), nil
}

func (s *fileStore) Set(key, value string) error {
	cfg, err := s.read()
	if err != nil {
		return err
	}
	if cfg.Keys == nil {
		cfg.Keys = map[string]string{}
	}
	cfg.Keys[key] = value
	return s.write(cfg)
}

func (s *fileStore) Delete(key string) error {
	cfg, err := s.read()
	if err != nil {
		return err
	}
	delete(cfg.Keys, key)
	return s.write(cfg)
}

func (s *fileStore) read() (fileConfig, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileConfig{Version: 1, Keys: map[string]string{}}, nil
		}
		return fileConfig{}, err
	}
	var cfg fileConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return fileConfig{}, err
	}
	if cfg.Keys == nil {
		cfg.Keys = map[string]string{}
	}
	return cfg, nil
}

func (s *fileStore) write(cfg fileConfig) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return fmt.Errorf("chmod config: %w", err)
	}
	return nil
}
