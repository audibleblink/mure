//go:build !linux && !darwin

package daemon

import "net"

// checkSelf returns ErrPeerAuthUnsupported on platforms without a supported
// peer-credential syscall (PRD §6: FreeBSD/others rejected at runtime).
func checkSelf(conn net.Conn) error {
	return ErrPeerAuthUnsupported
}
