package daemon

import (
	"errors"
	"net"
)

// ErrPeerAuthUnsupported is returned by Check on platforms that lack a
// peer-credential syscall we support (e.g. FreeBSD).
var ErrPeerAuthUnsupported = errors.New("peerauth: unsupported platform")

// ErrPeerNotSelf is returned by Check when the peer UID differs from os.Getuid().
var ErrPeerNotSelf = errors.New("peerauth: peer uid differs from self")

// CheckFunc is the signature of the peer-auth check. Exposed as a variable so
// tests can substitute a mock.
type CheckFunc func(conn net.Conn) error

// Check is the peer-auth entrypoint; set by platform build files.
var Check CheckFunc = checkSelf
