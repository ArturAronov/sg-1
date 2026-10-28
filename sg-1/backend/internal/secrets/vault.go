package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
)

type Vault struct{ aead cipher.AEAD }

func NewVault(encodedKey string) (*Vault, error) {
	key, err := base64.StdEncoding.Strict().DecodeString(encodedKey)
	if err != nil || len(key) != 32 {
		return nil, errors.New("SG1_CREDENTIALS_KEY must be a base64-encoded 32-byte encryption key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Vault{aead: aead}, nil
}

func associatedData(userID, provider string) []byte {
	return []byte("sg1/llm-key/v1\x00" + userID + "\x00" + provider)
}

// Seal binds a versioned encrypted value to its owner and provider. Moving a
// ciphertext to another user's record does not make it usable there.
func (v *Vault) Seal(userID, provider, key string) ([]byte, error) {
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	envelope := append([]byte{1}, nonce...)
	return v.aead.Seal(envelope, nonce, []byte(key), associatedData(userID, provider)), nil
}

// Open is for future trusted server-side provider calls, never a browser API.
func (v *Vault) Open(userID, provider string, envelope []byte) ([]byte, error) {
	n := v.aead.NonceSize()
	if len(envelope) < 1+n+v.aead.Overhead() || envelope[0] != 1 {
		return nil, errors.New("invalid credential envelope")
	}
	plaintext, err := v.aead.Open(nil, envelope[1:1+n], envelope[1+n:], associatedData(userID, provider))
	if err != nil {
		return nil, errors.New("credential could not be decrypted")
	}
	return plaintext, nil
}
