package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/phone"
	"github.com/hamzayogurtcuoglu/i-mux/internal/qr"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:    "phone",
		Summary: "cmd.phone",
		Args:    "[on | off | pair | devices [--json] | readonly <phone> on|off | forget <phone> | relay [<url> --secret S | off]] [--popup]",
		Run:     runPhone,
	})
	register(&command{
		Name:   "__phone-serve",
		Hidden: true,
		Run:    runPhoneServe,
	})
}

const phoneUsage = "phone [on | off | pair | devices [--json] | readonly <phone> on|off | forget <phone> | relay [<url> --secret S | off]] [--popup]"

// runPhone is the workspace from a phone (D104): its server on and off,
// pairing a phone with a QR code, and the paired phones.
func runPhone(e *Env, args []string) error {
	fs := newFlags(e, "phone")
	popup := fs.Bool("popup", false, "the Phone popup (the rail's Phone)")
	asJSON := fs.Bool("json", false, "devices: print JSON")
	secret := fs.String("secret", "", "relay: the relay's secret (imux-relay prints it)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	st := phone.DefaultStore()
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if *popup {
		return phonePopup(e, st, self)
	}
	usage := &UsageError{Msg: i18n.T("cli.usage", phoneUsage)}
	if len(pos) == 0 {
		phoneStatus(e, st)
		return nil
	}
	switch pos[0] {
	case "on":
		return phoneOn(e, st, self)
	case "off":
		phoneOff(st)
		fmt.Fprintln(e.Stdout, i18n.T("phone.turned_off"))
		return nil
	case "pair":
		return phonePair(e, st, self, false)
	case "devices":
		ds, err := st.Devices()
		if err != nil {
			return err
		}
		if *asJSON {
			for i := range ds {
				ds[i].Key = "" // never printed
			}
			if ds == nil {
				ds = []phone.Device{}
			}
			enc := json.NewEncoder(e.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(ds)
		}
		phoneDevices(e, ds)
		return nil
	case "readonly":
		if len(pos) != 3 || (pos[2] != "on" && pos[2] != "off") {
			return usage
		}
		d, err := findDevice(st, pos[1])
		if err != nil {
			return err
		}
		return st.SetReadOnly(d.ID, pos[2] == "on")
	case "forget":
		if len(pos) != 2 {
			return usage
		}
		d, err := findDevice(st, pos[1])
		if err != nil {
			return err
		}
		if err := st.Forget(d.ID); err != nil {
			return err
		}
		fmt.Fprintln(e.Stdout, i18n.T("phone.forgotten", d.Name))
		return nil
	case "relay":
		switch {
		case len(pos) == 1:
			phoneRelayStatus(e, st)
			return nil
		case len(pos) == 2 && pos[1] == "off":
			if err := st.ClearRelay(); err != nil {
				return err
			}
			fmt.Fprintln(e.Stdout, i18n.T("phone.relay_off"))
			return nil
		case len(pos) == 2:
			if *secret == "" {
				return &UsageError{Msg: i18n.T("phone.relay_needs_secret")}
			}
			return phoneSetRelay(e, st, self, pos[1], *secret)
		}
		return usage
	}
	return usage
}

// phoneSetRelay checks the relay, saves it and turns the server on, which
// dials it (D115).
func phoneSetRelay(e *Env, st *phone.Store, self, rawURL, secret string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := phone.CheckRelay(ctx, rawURL, secret); err != nil {
		return err
	}
	r, err := st.SetRelay(rawURL, secret)
	if err != nil {
		return err
	}
	if err := st.SetOn(true); err != nil {
		return err
	}
	if _, err := st.Ensure(self); err != nil {
		return err
	}
	fmt.Fprintln(e.Stdout, i18n.T("phone.relay_set", r.URL))
	// The server reads the relay within seconds; say how it went.
	for i := 0; i < 40; i++ {
		if s := st.RelayState().State; s == "connected" || s == "refused" {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	phoneRelayStatus(e, st)
	return nil
}

// phoneRelayStatus is the relay's line: its address and state, or how to
// set one up.
func phoneRelayStatus(e *Env, st *phone.Store) {
	r := st.Relay()
	if r == nil {
		fmt.Fprintln(e.Stdout, i18n.T("phone.relay_none"))
		return
	}
	state := i18n.T("phone.relay_connecting")
	rs := st.RelayState()
	switch {
	case st.Running() == nil:
		state = i18n.T("phone.relay_server_off")
	case rs.State == "connected":
		state = i18n.T("phone.relay_connected")
	case rs.State == "refused":
		state = i18n.T("phone.relay_refused", rs.Error)
	case rs.Error != "":
		state = i18n.T("phone.relay_connecting") + " " + rs.Error
	}
	fmt.Fprintf(e.Stdout, "  %s/r/%s/  (%s: %s)\n", r.URL, r.Room, i18n.T("phone.via_relay"), state)
}

// runPhoneServe is the server (`imux __phone-serve`, started by `imux phone
// on`).
func runPhoneServe(_ *Env, _ []string) error {
	eng := &phone.TmuxEngine{T: tmux.New(paths.TmuxConf())}
	// The desktop's theme, read at each phone's connection (D110).
	look := func() phone.Look {
		cfg, _, err := config.Load(paths.Config())
		if err != nil {
			return phone.Look{}
		}
		return phone.LookOf(cfg.Theme)
	}
	return phone.Serve(context.Background(), phone.DefaultStore(), eng, look)
}

func phoneOn(e *Env, st *phone.Store, self string) error {
	if err := st.SetOn(true); err != nil {
		return err
	}
	info, err := st.Ensure(self)
	if err != nil {
		return err
	}
	fmt.Fprintln(e.Stdout, i18n.T("phone.turned_on"))
	printAddresses(e, info.Port)
	return nil
}

func phoneOff(st *phone.Store) {
	_ = st.SetOn(false)
	st.CancelPairing()
	st.StopServer()
}

// EnsurePhone brings the phone server back when it is on but not running
// (a restart of the computer): imux's start calls it.
func EnsurePhone() {
	st := phone.DefaultStore()
	if !st.Settings().On || st.Running() != nil {
		return
	}
	if self, err := os.Executable(); err == nil {
		_, _ = st.Ensure(self)
	}
}

func printAddresses(e *Env, port int) {
	addrs := phone.Addresses()
	if len(addrs) == 0 {
		fmt.Fprintln(e.Stdout, "  "+i18n.T("phone.no_network"))
		return
	}
	for _, a := range addrs {
		via := i18n.T("phone.via_lan")
		if a.Kind == "tailscale" {
			via = i18n.T("phone.via_tailscale")
		}
		fmt.Fprintf(e.Stdout, "  http://%s:%d  (%s)\n", a.IP, port, via)
	}
}

func phoneStatus(e *Env, st *phone.Store) {
	if i := st.Running(); i != nil {
		fmt.Fprintln(e.Stdout, i18n.T("phone.status_on"))
		printAddresses(e, i.Port)
		if st.Relay() != nil {
			phoneRelayStatus(e, st)
		}
	} else {
		fmt.Fprintln(e.Stdout, i18n.T("phone.status_off"))
	}
	ds, _ := st.Devices()
	fmt.Fprintln(e.Stdout)
	phoneDevices(e, ds)
}

func phoneDevices(e *Env, ds []phone.Device) {
	if len(ds) == 0 {
		fmt.Fprintln(e.Stdout, i18n.T("phone.no_devices"))
		return
	}
	fmt.Fprintln(e.Stdout, i18n.T("phone.devices"))
	tw := tabwriter.NewWriter(e.Stdout, 0, 0, 2, ' ', 0)
	for i, d := range ds {
		seen := i18n.T("phone.never")
		if d.Seen > 0 {
			seen = ago(time.Unix(d.Seen, 0))
		}
		ro := ""
		if d.ReadOnly {
			ro = i18n.T("phone.read_only")
		}
		fmt.Fprintf(tw, "  %d\t%s\t%s\t%s\n", i+1, d.Name, i18n.T("phone.seen", seen), ro)
	}
	_ = tw.Flush()
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return i18n.T("phone.just_now")
	case d < time.Hour:
		return i18n.T("phone.minutes_ago", int(d/time.Minute))
	case d < 48*time.Hour:
		return i18n.T("phone.hours_ago", int(d/time.Hour))
	}
	return i18n.T("phone.days_ago", int(d/(24*time.Hour)))
}

// findDevice is a device by its number in the list, its name or its id.
func findDevice(st *phone.Store, which string) (*phone.Device, error) {
	ds, err := st.Devices()
	if err != nil {
		return nil, err
	}
	if n, err := strconv.Atoi(which); err == nil && n >= 1 && n <= len(ds) {
		return &ds[n-1], nil
	}
	for i := range ds {
		if ds[i].ID == which || strings.EqualFold(ds[i].Name, which) {
			return &ds[i], nil
		}
	}
	return nil, fmt.Errorf("%s", i18n.T("phone.no_such", which))
}

// phonePair turns the server on, makes a pairing and shows its QR code,
// then waits for the phone (or for the code to expire).
func phonePair(e *Env, st *phone.Store, self string, quiet bool) error {
	if err := st.SetOn(true); err != nil {
		return err
	}
	info, err := st.Ensure(self)
	if err != nil {
		return err
	}
	// Through the relay when one is set up (D115), else the LAN's address
	// (Tailscale's first: it reaches the phone anywhere).
	relay := st.Relay()
	addrs := phone.Addresses()
	if relay == nil && len(addrs) == 0 {
		return fmt.Errorf("%s", i18n.T("phone.no_network"))
	}
	before, _ := st.Devices()
	p, err := st.NewPairing(time.Now())
	if err != nil {
		return err
	}
	var u, note string
	switch {
	case relay != nil:
		u, note = phone.RelayPairURL(relay, p), i18n.T("phone.relay_pair")
	case addrs[0].Kind == "lan":
		u, note = phone.PairURL(addrs[0].IP, info.Port, p), i18n.T("phone.lan_only")
	default:
		u, note = phone.PairURL(addrs[0].IP, info.Port, p), i18n.T("phone.tailscale")
	}
	code, err := qr.Encode(u)
	if err != nil {
		return err
	}
	fmt.Fprint(e.Stdout, code.Terminal())
	fmt.Fprintln(e.Stdout)
	fmt.Fprintln(e.Stdout, i18n.T("phone.scan", int(phone.PairFor/time.Minute)))
	fmt.Fprintln(e.Stdout, note)
	if quiet {
		return nil
	}
	deadline := time.Now().Add(phone.PairFor)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		if ds, _ := st.Devices(); len(ds) > len(before) {
			fmt.Fprintln(e.Stdout, "\n"+i18n.T("phone.paired", ds[len(ds)-1].Name))
			return nil
		}
	}
	st.CancelPairing()
	fmt.Fprintln(e.Stdout, "\n"+i18n.T("phone.expired"))
	return nil
}

// phonePopup is the rail's Phone: the server's state, the phones, and a
// letter for each thing to do.
func phonePopup(e *Env, st *phone.Store, self string) error {
	in := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprint(e.Stdout, "\x1b[H\x1b[2J")
		fmt.Fprintln(e.Stdout, i18n.T("phone.title")+"\n")
		phoneStatus(e, st)
		fmt.Fprintln(e.Stdout)
		on := st.Running() != nil
		toggle := i18n.T("phone.do_on")
		if on {
			toggle = i18n.T("phone.do_off")
		}
		fmt.Fprintln(e.Stdout, "  o  "+toggle)
		fmt.Fprintln(e.Stdout, "  p  "+i18n.T("phone.do_pair"))
		if ds, _ := st.Devices(); len(ds) > 0 {
			fmt.Fprintln(e.Stdout, "  r  "+i18n.T("phone.do_readonly"))
			fmt.Fprintln(e.Stdout, "  f  "+i18n.T("phone.do_forget"))
		}
		fmt.Fprint(e.Stdout, "\n"+i18n.T("phone.prompt"))
		line, err := in.ReadString('\n')
		f := strings.Fields(line)
		if err != nil || len(f) == 0 {
			return nil
		}
		var msg string
		switch f[0] {
		case "o":
			if on {
				phoneOff(st)
			} else if err := phoneOn(&Env{Stdout: discard{}, Stderr: e.Stderr}, st, self); err != nil {
				msg = err.Error()
			}
		case "p":
			fmt.Fprint(e.Stdout, "\x1b[H\x1b[2J")
			if err := phonePair(e, st, self, false); err != nil {
				msg = err.Error()
			} else {
				fmt.Fprint(e.Stdout, "\n"+i18n.T("phone.press_enter"))
				_, _ = in.ReadString('\n')
			}
		case "r", "f":
			if len(f) < 2 {
				msg = i18n.T("phone.which")
				break
			}
			d, err := findDevice(st, f[1])
			if err != nil {
				msg = err.Error()
				break
			}
			if f[0] == "r" {
				err = st.SetReadOnly(d.ID, !d.ReadOnly)
			} else {
				err = st.Forget(d.ID)
			}
			if err != nil {
				msg = err.Error()
			}
		default:
			msg = i18n.T("phone.unknown", f[0])
		}
		if msg != "" {
			fmt.Fprint(e.Stdout, "\n"+msg+"\n"+i18n.T("phone.press_enter"))
			_, _ = in.ReadString('\n')
		}
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// showPhone opens the Phone popup on client (the rail's Phone).
func showPhone(w *workspace.WS, client string) error {
	c := tmux.Cmd{"display-popup"}
	if client != "" {
		c = append(c, "-c", client)
	}
	c = append(c, "-E", "-w", "72", "-h", "90%")
	return w.T.Spawn(append(c, tmux.Program(w.Self, "phone", "--popup")...))
}
