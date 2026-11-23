//go:build !windows

package machine

// SharesConnection reports whether ssh keeps one master connection per
// machine that every command rides on (D73).
const SharesConnection = true
