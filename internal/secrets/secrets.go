package secrets

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

type Store interface {
	Kind() string
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

func NewStore(kind, service string) (Store, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "keychain":
		return newKeychainStore(service)
	case "file":
		return newFileStore(service)
	default:
		return nil, fmt.Errorf("unknown store: %s", kind)
	}
}

func ResolveAPIKey(flagValue string, fromStdin bool, envVarName string) (string, error) {
	if strings.TrimSpace(flagValue) != "" {
		return strings.TrimSpace(flagValue), nil
	}
	if fromStdin {
		b, err := io.ReadAll(io.LimitReader(os.Stdin, 64*1024))
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stderr, "Enter %s (input hidden): ", envVarName)
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	return "", errors.New("no API key provided (not a TTY; use --api-key-stdin)")
}
