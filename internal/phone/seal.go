// Package phone is the workspace from a phone (D104, M109): a server in
// imux that a paired phone's web app talks to, every message sealed with
// the key the two share since pairing.
package phone

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// Key is a device's 256-bit AES key, made at pairing and shown only in the
// QR code's URL fragment.
type Key [32]byte

var b64 = base64.RawURLEncoding

// NewKey makes a random key.
func NewKey() (Key, error) {
	var k Key
	_, err := rand.Read(k[:])
	return k, err
}

// String is the key in URL-safe base64, without padding.
func (k Key) String() string { return b64.EncodeToString(k[:]) }

// ParseKey reads a key String wrote.
func ParseKey(s string) (Key, error) {
	var k Key
	b, err := b64.DecodeString(s)
	if err != nil || len(b) != len(k) {
		return k, errors.New("phone: bad key")
	}
	copy(k[:], b)
	return k, nil
}

const nonceSize = 12

// Seal encrypts plain with k (AES-256-GCM, a random nonce, aad as
// additional data: the device's id, so a message is good for one device
// only) and returns nonce ‖ ciphertext ‖ tag in URL-safe base64.
func Seal(k Key, aad string, plain []byte) (string, error) {
	g, err := gcm(k)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, nonceSize, nonceSize+len(plain)+g.Overhead())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return b64.EncodeToString(g.Seal(nonce, nonce, plain, []byte(aad))), nil
}

// ErrSeal means a message was not sealed with the device's key.
var ErrSeal = errors.New("phone: message not sealed with this device's key")

// Open decrypts what Seal made, and returns the nonce too (for replays).
func Open(k Key, aad, sealed string) (plain []byte, nonce string, err error) {
	b, err := b64.DecodeString(sealed)
	if err != nil || len(b) < nonceSize+16 {
		return nil, "", ErrSeal
	}
	g, err := gcm(k)
	if err != nil {
		return nil, "", err
	}
	plain, err = g.Open(nil, b[:nonceSize], b[nonceSize:], []byte(aad))
	if err != nil {
		return nil, "", ErrSeal
	}
	return plain, string(b[:nonceSize]), nil
}

func gcm(k Key) (cipher.AEAD, error) {
	block, err := aes.NewCipher(k[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Window is how far a message's time may be from the server's.
const Window = time.Minute

// ErrStale means a message is too old (or from the future), or was seen
// before: a replay.
var ErrStale = errors.New("phone: stale or repeated message")

// Guard refuses replays: a message must carry a time within Window, and
// its nonce is accepted once.
type Guard struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

// Check accepts a message sealed with nonce at ts (unix ms) when now is
// within Window of it and the nonce is new.
func (g *Guard) Check(nonce string, ts int64, now time.Time) error {
	at := time.UnixMilli(ts)
	if at.Before(now.Add(-Window)) || at.After(now.Add(Window)) {
		return ErrStale
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.seen == nil {
		g.seen = map[string]time.Time{}
	}
	for n, t := range g.seen {
		if now.Sub(t) > 2*Window {
			delete(g.seen, n)
		}
	}
	if _, ok := g.seen[nonce]; ok {
		return ErrStale
	}
	g.seen[nonce] = now
	return nil
}

// Envelope is the part of every message from the phone the server checks
// first: its time.
type Envelope struct {
	TS int64 `json:"ts"`
}

// OpenChecked opens a phone's message with k, checks its time and nonce
// with g, and decodes it into v.
func OpenChecked(k Key, aad, sealed string, g *Guard, now time.Time, v any) error {
	plain, nonce, err := Open(k, aad, sealed)
	if err != nil {
		return err
	}
	var env Envelope
	if err := json.Unmarshal(plain, &env); err != nil {
		return ErrSeal
	}
	if err := g.Check(nonce, env.TS, now); err != nil {
		return err
	}
	if v != nil {
		return json.Unmarshal(plain, v)
	}
	return nil
}
