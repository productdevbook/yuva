package secret

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

const version byte = 1

// Key is the master key, and during a rotation the previous one: values are sealed and keys
// derived under the current key, and values sealed under either key open.
type Key struct {
	cur  *single
	prev *single
}

type single struct {
	aead cipher.AEAD
	raw  []byte
}

func parseSingle(s string) (*single, error) {
	s = strings.TrimSpace(s)
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		raw, err = base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
	}
	if err != nil || len(raw) != 32 {
		return nil, errors.New("the master key must be 32 bytes, base64-encoded")
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &single{aead: aead, raw: raw}, nil
}

func ParseKey(s string) (*Key, error) {
	cur, err := parseSingle(s)
	if err != nil {
		return nil, err
	}
	return &Key{cur: cur}, nil
}

// ParseKeys reads the current key and, when previous is not empty, the key being retired.
func ParseKeys(current, previous string) (*Key, error) {
	k, err := ParseKey(current)
	if err != nil || strings.TrimSpace(previous) == "" {
		return k, err
	}
	prev, err := parseSingle(previous)
	if err != nil {
		return nil, errors.New("the previous master key must be 32 bytes, base64-encoded")
	}
	if bytes.Equal(prev.raw, k.cur.raw) {
		return nil, errors.New("the previous master key is the current one")
	}
	k.prev = prev
	return k, nil
}

func (k *Key) HasPrevious() bool { return k.prev != nil }

func (k *Key) Seal(plaintext, context []byte) []byte {
	return k.cur.seal(plaintext, context)
}

func (s *single) seal(plaintext, context []byte) []byte {
	nonce := make([]byte, s.aead.NonceSize())
	_, _ = rand.Read(nonce)
	out := append([]byte{version}, nonce...)
	return s.aead.Seal(out, nonce, plaintext, context)
}

func (s *single) open(sealed, context []byte) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(sealed) < 1+n || sealed[0] != version {
		return nil, errors.New("secret: unknown format")
	}
	return s.aead.Open(nil, sealed[1:1+n], sealed[1+n:], context)
}

func (k *Key) Open(sealed, context []byte) ([]byte, error) {
	plain, err := k.cur.open(sealed, context)
	if err != nil && k.prev != nil {
		if p, perr := k.prev.open(sealed, context); perr == nil {
			return p, nil
		}
	}
	return plain, err
}

// Reseal returns sealed unchanged when it opens with the current key, and sealed again under the
// current key with stale set when it opens only with the previous one.
func (k *Key) Reseal(sealed, context []byte) (out []byte, stale bool, err error) {
	if _, err := k.cur.open(sealed, context); err == nil {
		return sealed, false, nil
	}
	if k.prev == nil {
		return nil, false, errors.New("secret: does not open with the master key")
	}
	plain, err := k.prev.open(sealed, context)
	if err != nil {
		return nil, false, errors.New("secret: opens with neither master key")
	}
	return k.cur.seal(plain, context), true, nil
}

// Derive returns a 32-byte key for one purpose, named by info, derived from the master key with
// HKDF-SHA256, so a key used for hashing never doubles as the encryption key.
func (k *Key) Derive(info string) []byte {
	return k.cur.derive(info)
}

// Derived is the key for info under the current master key, then under the previous one, for
// checking values made before a rotation.
func (k *Key) Derived(info string) [][]byte {
	out := [][]byte{k.cur.derive(info)}
	if k.prev != nil {
		out = append(out, k.prev.derive(info))
	}
	return out
}

func (s *single) derive(info string) []byte {
	out, err := hkdf.Key(sha256.New, s.raw, nil, info, 32)
	if err != nil {
		panic(err)
	}
	return out
}
