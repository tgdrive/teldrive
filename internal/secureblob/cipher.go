package secureblob

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

var (
	ErrInvalidKey        = errors.New("secure blob key must decode to 32 bytes")
	ErrInvalidCiphertext = errors.New("secure blob ciphertext is invalid")
)

// Cipher encrypts sensitive database fields with XChaCha20-Poly1305. Purpose is
// authenticated as associated data so a token, bot credential, login state, or
// Telegram session cannot be replayed in another column.
type Cipher struct {
	key    [chacha20poly1305.KeySize]byte
	random io.Reader
}

func New(base64Key string) (*Cipher, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(base64Key)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(base64Key)
	}
	if err != nil || len(decoded) != chacha20poly1305.KeySize {
		return nil, ErrInvalidKey
	}
	cipher := &Cipher{random: rand.Reader}
	copy(cipher.key[:], decoded)
	return cipher, nil
}

func NewWithKey(key []byte, random io.Reader) (*Cipher, error) {
	if len(key) != chacha20poly1305.KeySize {
		return nil, ErrInvalidKey
	}
	if random == nil {
		return nil, errors.New("secure blob random source is required")
	}
	cipher := &Cipher{random: random}
	copy(cipher.key[:], key)
	return cipher, nil
}

func (c *Cipher) Seal(purpose string, plaintext []byte) ([]byte, error) {
	if c == nil || c.random == nil || purpose == "" {
		return nil, ErrInvalidKey
	}
	aead, err := chacha20poly1305.NewX(c.key[:])
	if err != nil {
		return nil, fmt.Errorf("create secure blob cipher: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(c.random, nonce); err != nil {
		return nil, fmt.Errorf("read secure blob nonce: %w", err)
	}
	out := make([]byte, 1+len(nonce), 1+len(nonce)+len(plaintext)+aead.Overhead())
	out[0] = 1
	copy(out[1:], nonce)
	return aead.Seal(out, nonce, plaintext, []byte(purpose)), nil
}

func (c *Cipher) Open(purpose string, ciphertext []byte) ([]byte, error) {
	if c == nil || purpose == "" {
		return nil, ErrInvalidKey
	}
	aead, err := chacha20poly1305.NewX(c.key[:])
	if err != nil {
		return nil, fmt.Errorf("create secure blob cipher: %w", err)
	}
	if len(ciphertext) < 1+aead.NonceSize()+aead.Overhead() || ciphertext[0] != 1 {
		return nil, ErrInvalidCiphertext
	}
	nonce := ciphertext[1 : 1+aead.NonceSize()]
	plaintext, err := aead.Open(nil, nonce, ciphertext[1+aead.NonceSize():], []byte(purpose))
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	return plaintext, nil
}

// Encrypt implements RiverPro's job argument encryptor using a dedicated
// authenticated-data purpose. RiverPro's Encryptor contract cannot return an
// error, so encryption failures follow its reference implementation and panic.
func (c *Cipher) Encrypt(plaintext []byte) []byte {
	ciphertext, err := c.Seal("river-job-args", plaintext)
	if err != nil {
		panic(fmt.Sprintf("encrypt River job arguments: %v", err))
	}
	return ciphertext
}

// Decrypt implements RiverPro's job argument decryptor.
func (c *Cipher) Decrypt(ciphertext []byte) ([]byte, error) {
	return c.Open("river-job-args", ciphertext)
}
