//go:build linux

package daemon

import (
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// checkSelf verifies the peer's UID equals our own via SO_PEERCRED.
func checkSelf(conn net.Conn) error {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return ErrPeerAuthUnsupported
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return err
	}
	var ucred *unix.Ucred
	var sockErr error
	cerr := raw.Control(func(fd uintptr) {
		ucred, sockErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if cerr != nil {
		return cerr
	}
	if sockErr != nil {
		return sockErr
	}
	if int(ucred.Uid) != os.Getuid() {
		return ErrPeerNotSelf
	}
	return nil
}
