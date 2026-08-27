//go:build !unix

// Identity-capture fallback for platforms without syscall.Stat_t device and
// inode semantics. Sqloid's request-boundary identity contract targets Linux
// and macOS; on other platforms nothing is recorded and every verification
// trivially passes so sessions keep working without the Linux/macOS checks.

package connection

import "errors"

// statIdentitySupported reports that device/inode identity checks are not
// available on this platform.
func statIdentitySupported() bool { return false }

func statIdentity(path string) (dev, ino uint64, err error) {
	return 0, 0, errors.New("device and inode identity checks require linux or darwin")
}
