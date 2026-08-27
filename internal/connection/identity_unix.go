//go:build unix

// Unix identity capture for Issue #7: the device and inode of a validated
// database path are the only stable identifiers Sqloid compares at request
// boundaries on Linux and macOS.

package connection

import (
	"fmt"
	"os"
	"syscall"
)

// statIdentitySupported reports that this platform can capture and compare
// device/inode pairs.
func statIdentitySupported() bool { return true }

// statIdentity reports the device and inode of path exactly as the kernel
// sees them, so comparison against a recorded reference cannot be fooled by a
// same-named file with different filesystem identity.
func statIdentity(path string) (dev, ino uint64, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return 0, 0, fmt.Errorf("stat %s: unsupported system stat type %T", path, info.Sys())
	}
	return uint64(st.Dev), uint64(st.Ino), nil
}
