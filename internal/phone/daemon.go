package phone

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
	"github.com/hamzayogurtcuoglu/i-mux/internal/version"
)

// Port is where the server listens first (then the next free of 20).
const Port = 7655

// Info is the running server's record.
type Info struct {
	PID  int `json:"pid"`
	Port int `json:"port"`
}

// Running is the server's record when its process is alive, else nil.
func (s *Store) Running() *Info {
	var i Info
	if err := s.read("server.json", &i); err != nil || i.PID == 0 || !sysx.Alive(i.PID) {
		return nil
	}
	return &i
}

// Settings are the phone's settings: whether the server is on (it comes
// back when imux starts while it is).
type Settings struct {
	On bool `json:"on"`
}

// Settings reads them (off when never set).
func (s *Store) Settings() Settings {
	var st Settings
	_ = s.read("settings.json", &st)
	return st
}

// SetOn records whether the server should run.
func (s *Store) SetOn(on bool) error { return s.write("settings.json", Settings{On: on}) }

// Serve runs the server until a stop signal: on every interface (the
// phone is on the LAN or Tailscale), at Port or the next free one,
// recorded in server.json. look is the desktop's theme (D110).
func Serve(ctx context.Context, st *Store, eng Engine, look func() Look) error {
	ln, err := listen(Port)
	if err != nil {
		return err
	}
	info := Info{PID: os.Getpid(), Port: ln.Addr().(*net.TCPAddr).Port}
	if err := st.write("server.json", info); err != nil {
		_ = ln.Close()
		return err
	}
	defer func() {
		var cur Info
		if st.read("server.json", &cur) == nil && cur.PID == os.Getpid() {
			_ = os.Remove(st.file("server.json"))
		}
	}()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sig := make(chan os.Signal, 1)
	sysx.Notify(sig, sysx.Std()...)
	defer sysx.Stop(sig)
	go func() {
		select {
		case <-sig:
			cancel()
		case <-ctx.Done():
		}
	}()
	srv := &Server{Store: st, Engine: eng, Look: look, Done: make(chan struct{})}
	h := srv.Handler()
	hs := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	// The relay, when one is set up (D115): the same handler, from anywhere.
	ver, _, _ := version.Info()
	go RunRelay(ctx, st, h, ver, nil)
	go func() {
		<-ctx.Done()
		close(srv.Done) // ends the event streams
		sctx, c := context.WithTimeout(context.Background(), 2*time.Second)
		defer c()
		_ = hs.Shutdown(sctx)
	}()
	if err := hs.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func listen(port int) (net.Listener, error) {
	for p := port; p < port+20; p++ {
		if ln, err := net.Listen("tcp", ":"+strconv.Itoa(p)); err == nil {
			return ln, nil
		}
	}
	return net.Listen("tcp", ":0")
}

// Ensure starts the server (`self __phone-serve`, detached) unless it runs.
func (s *Store) Ensure(self string) (*Info, error) {
	if i := s.Running(); i != nil {
		return i, nil
	}
	if err := paths.EnsureDir(s.Dir); err != nil {
		return nil, err
	}
	logf, err := os.OpenFile(filepath.Join(s.Dir, "serve.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, paths.FileMode)
	if err != nil {
		return nil, err
	}
	defer logf.Close()
	cmd, err := sysx.StartDetachedCmd(func() *exec.Cmd {
		c := exec.Command(self, "__phone-serve")
		c.Stdin, c.Stdout, c.Stderr = nil, nil, logf
		return c
	})
	if err != nil {
		return nil, err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-exited:
			return nil, fmt.Errorf("the phone server stopped: %v (see %s)", err, paths.Shorten(logf.Name()))
		case <-time.After(30 * time.Millisecond):
		}
		if i := s.Running(); i != nil && i.PID == cmd.Process.Pid {
			return i, nil
		}
	}
	_ = cmd.Process.Kill()
	return nil, errors.New("the phone server did not start")
}

// StopServer ends the running server, if any.
func (s *Store) StopServer() {
	if i := s.Running(); i != nil {
		_ = sysx.Signal(i.PID, sysx.SIGTERM)
		for n := 0; n < 100 && sysx.Alive(i.PID); n++ {
			time.Sleep(20 * time.Millisecond)
		}
	}
}

// Address is a way the phone can reach this computer.
type Address struct {
	Kind string // "tailscale" or "lan"
	IP   string
}

// Addresses are this computer's Tailscale (100.64.0.0/10) and private LAN
// IPv4 addresses, Tailscale first: it reaches the phone anywhere.
func Addresses() []Address {
	var ts, lan []Address
	_, tsNet, _ := net.ParseCIDR("100.64.0.0/10")
	ifs, _ := net.Interfaces()
	for _, in := range ifs {
		if in.Flags&net.FlagUp == 0 || in.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := in.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipn.IP.To4()
			if ip == nil {
				continue
			}
			switch {
			case tsNet.Contains(ip):
				ts = append(ts, Address{Kind: "tailscale", IP: ip.String()})
			case ip.IsPrivate():
				lan = append(lan, Address{Kind: "lan", IP: ip.String()})
			}
		}
	}
	return append(ts, lan...)
}

// PairURL is the address a QR code shows: the app at ip:port, with the
// pairing's token and key in the fragment, which browsers never send.
func PairURL(ip string, port int, p Pending) string {
	return "http://" + net.JoinHostPort(ip, strconv.Itoa(port)) + "/#p=" + p.Token + "." + p.Key
}
