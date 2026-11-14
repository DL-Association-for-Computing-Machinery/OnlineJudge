package redisstore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
)

// NewReviewCursorCipher derives a dedicated key from the deployment's stable
// login admission secret, without exposing it or reusing it directly as a key.
func NewReviewCursorCipher(secrets *LoginSecrets) (cipher.AEAD, error) {
	if !validLoginSecrets(secrets) {
		return nil, ErrLoginSecrets
	}
	key, err := hkdf.Key(sha256.New, secrets.limiting[:], nil, "oj.review-discovery-cursor.v1", 32)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}
