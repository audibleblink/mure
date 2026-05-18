//go:build darwin

package daemon

import (
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// checkSelf verifies the peer's UID equals our own via LOCAL_PEERCRED.
func checkSelf(conn net.Conn) error {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return ErrPeerAuthUnsupported
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return err
	}
	var cred *unix.Xucred
	var sockErr error
	cerr := raw.Control(func(fd uintptr) {
		cred, sockErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	})
	if cerr != nil {
		return cerr
	}
	if sockErr != nil {
		return sockErr
	}
	if int(cred.Uid) != os.Getuid() {
		return ErrPeerNotSelf
	}
	return nil
}
