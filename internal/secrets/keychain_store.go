package secrets

import (
	"github.com/zalando/go-keyring"
)

type keychainStore struct {
	service string
}

func newKeychainStore(service string) (Store, error) {
	return &keychainStore{service: service}, nil
}

func (s *keychainStore) Kind() string { return "keychain" }

func (s *keychainStore) Get(key string) (string, error) {
	return keyring.Get(s.service, key)
}

func (s *keychainStore) Set(key, value string) error {
	return keyring.Set(s.service, key, value)
}

func (s *keychainStore) Delete(key string) error {
	return keyring.Delete(s.service, key)
}
