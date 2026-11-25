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
		Args:    "[on | off | pair | devices [--json] | readonly <phone> on|off | forget <phone>] [--popup]",
		Run:     runPhone,
	})
	register(&command{
		Name:   "__phone-serve",
		Hidden: true,
		Run:    runPhoneServe,
	})
}

const phoneUsage = "phone [on | off | pair | devices [--json] | readonly <phone> on|off | forget <phone>] [--popup]"

// runPhone is the workspace from a phone (D104): its server on and off,
// pairing a phone with a QR code, and the paired phones.
func runPhone(e *Env, args []string) error {
	fs := newFlags(e, "phone")
	popup := fs.Bool("popup", false, "the Phone popup (the rail's Phone)")
	asJSON := fs.Bool("json", false, "devices: print JSON")
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
	}
	return usage
}

// runPhoneServe is the server (`imux __phone-serve`, started by `imux phone
// on`).
func runPhoneServe(_ *Env, _ []string) error {
	eng := &phone.TmuxEngine{T: tmux.New(paths.TmuxConf())}
	return phone.Serve(context.Background(), phone.DefaultStore(), eng)
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
	addrs := phone.Addresses()
	if len(addrs) == 0 {
		return fmt.Errorf("%s", i18n.T("phone.no_network"))
	}
	a := addrs[0] // Tailscale first: it reaches the phone anywhere
	before, _ := st.Devices()
	p, err := st.NewPairing(time.Now())
	if err != nil {
		return err
	}
	code, err := qr.Encode(phone.PairURL(a.IP, info.Port, p))
	if err != nil {
		return err
	}
	fmt.Fprint(e.Stdout, code.Terminal())
	fmt.Fprintln(e.Stdout)
	fmt.Fprintln(e.Stdout, i18n.T("phone.scan", int(phone.PairFor/time.Minute)))
	if a.Kind == "lan" {
		fmt.Fprintln(e.Stdout, i18n.T("phone.lan_only"))
	} else {
		fmt.Fprintln(e.Stdout, i18n.T("phone.tailscale"))
	}
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
