package phone

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
)

// Device is a paired phone.
type Device struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Key      string `json:"key"`
	ReadOnly bool   `json:"read_only,omitempty"`
	Added    int64  `json:"added"`          // unix seconds
	Seen     int64  `json:"seen,omitempty"` // unix seconds of its last message
}

// Pending is a pairing the QR code offers: its token and key, until
// Expires (unix seconds); it is used once.
type Pending struct {
	Token   string `json:"token"`
	Key     string `json:"key"`
	Expires int64  `json:"expires"`
}

// PairFor is how long a QR code can pair.
const PairFor = 2 * time.Minute

// Store keeps the devices and the pending pairing in Dir (0700, files
// 0600). The server reads it at every request, so `imux phone forget` and
// `readonly` take effect at once.
type Store struct{ Dir string }

// DefaultStore is ~/.imux/phone.
func DefaultStore() *Store { return &Store{Dir: filepath.Join(paths.Home(), "phone")} }

func (s *Store) file(name string) string { return filepath.Join(s.Dir, name) }

func (s *Store) read(name string, v any) error {
	b, err := os.ReadFile(s.file(name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (s *Store) write(name string, v any) error {
	if err := paths.EnsureDir(s.Dir); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return paths.WriteFile(s.file(name), b)
}

// Devices lists the paired devices.
func (s *Store) Devices() ([]Device, error) {
	var ds []Device
	err := s.read("devices.json", &ds)
	return ds, err
}

// Device is the paired device id, or nil.
func (s *Store) Device(id string) *Device {
	ds, err := s.Devices()
	if err != nil {
		return nil
	}
	for i := range ds {
		if subtle.ConstantTimeCompare([]byte(ds[i].ID), []byte(id)) == 1 {
			return &ds[i]
		}
	}
	return nil
}

func (s *Store) update(id string, f func(*Device)) error {
	ds, err := s.Devices()
	if err != nil {
		return err
	}
	for i := range ds {
		if ds[i].ID == id {
			f(&ds[i])
			return s.write("devices.json", ds)
		}
	}
	return ErrNoDevice
}

// ErrNoDevice means no paired device has that id.
var ErrNoDevice = errors.New("phone: no such device")

// Forget removes a device: its key opens nothing any more.
func (s *Store) Forget(id string) error {
	ds, err := s.Devices()
	if err != nil {
		return err
	}
	for i := range ds {
		if ds[i].ID == id {
			return s.write("devices.json", append(ds[:i], ds[i+1:]...))
		}
	}
	return ErrNoDevice
}

// SetReadOnly makes a device only watch, or lets it send again.
func (s *Store) SetReadOnly(id string, ro bool) error {
	return s.update(id, func(d *Device) { d.ReadOnly = ro })
}

// Touch records a device's last message.
func (s *Store) Touch(id string, now time.Time) {
	_ = s.update(id, func(d *Device) { d.Seen = now.Unix() })
}

// NewPairing makes the pairing a QR code offers, replacing any other.
func (s *Store) NewPairing(now time.Time) (Pending, error) {
	k, err := NewKey()
	if err != nil {
		return Pending{}, err
	}
	p := Pending{Token: randomID(16), Key: k.String(), Expires: now.Add(PairFor).Unix()}
	return p, s.write("pending.json", p)
}

// ErrPairing means the token is not the pending pairing's, or it expired
// or was used.
var ErrPairing = errors.New("phone: this pairing code is not valid any more")

// Pair makes the device the pending pairing names, once and in time.
func (s *Store) Pair(token, name string, now time.Time) (Device, error) {
	var p Pending
	if err := s.read("pending.json", &p); err != nil || p.Token == "" {
		return Device{}, ErrPairing
	}
	if subtle.ConstantTimeCompare([]byte(p.Token), []byte(token)) != 1 || now.Unix() > p.Expires {
		return Device{}, ErrPairing
	}
	_ = os.Remove(s.file("pending.json")) // once
	if len(name) > 60 {
		name = name[:60]
	}
	d := Device{ID: randomID(12), Name: name, Key: p.Key, Added: now.Unix()}
	ds, err := s.Devices()
	if err != nil {
		return Device{}, err
	}
	return d, s.write("devices.json", append(ds, d))
}

// CancelPairing drops the pending pairing.
func (s *Store) CancelPairing() { _ = os.Remove(s.file("pending.json")) }

func randomID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b64.EncodeToString(b)
}
