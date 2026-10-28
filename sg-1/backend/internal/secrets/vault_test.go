package secrets

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestVault(t *testing.T) {
	v, err := NewVault(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	key := "synthetic-test-api-key-not-a-real-token"
	one, err := v.Seal("user-a", "openai", key)
	if err != nil {
		t.Fatal(err)
	}
	two, err := v.Seal("user-a", "openai", key)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(one, two) || bytes.Contains(one, []byte(key)) {
		t.Fatal("ciphertext must be randomized and not contain plaintext")
	}
	plain, err := v.Open("user-a", "openai", one)
	if err != nil || string(plain) != key {
		t.Fatal("round-trip failed", err)
	}
	for _, scope := range [][2]string{{"user-b", "openai"}, {"user-a", "anthropic"}} {
		if _, err := v.Open(scope[0], scope[1], one); err == nil {
			t.Fatal("ciphertext was usable outside its owner/provider")
		}
	}
	tampered := bytes.Clone(one)
	tampered[len(tampered)-1] ^= 1
	for _, bad := range [][]byte{nil, {1, 2, 3}, tampered, append([]byte{2}, one[1:]...)} {
		if _, err := v.Open("user-a", "openai", bad); err == nil {
			t.Fatal("invalid envelope accepted")
		}
	}
	other, _ := NewVault(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32)))
	if _, err := other.Open("user-a", "openai", one); err == nil {
		t.Fatal("wrong encryption key accepted")
	}
}

func TestInvalidVaultKeys(t *testing.T) {
	for _, key := range []string{"", "not-base64", base64.StdEncoding.EncodeToString(make([]byte, 16)), base64.StdEncoding.EncodeToString(make([]byte, 64))} {
		if _, err := NewVault(key); err == nil {
			t.Fatal("invalid master key accepted")
		}
	}
}
