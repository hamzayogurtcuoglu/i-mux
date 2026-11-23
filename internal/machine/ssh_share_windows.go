//go:build windows

package machine

// SharesConnection reports whether ssh keeps one master connection per
// machine that every command rides on. Not on Windows (D96): its OpenSSH
// has no Unix-socket multiplexing, and with ControlMaster every ssh fails at
// once ("getsockname failed: Not a socket"). Each command connects on its
// own, so a machine needs SSH key sign-in.
const SharesConnection = false
