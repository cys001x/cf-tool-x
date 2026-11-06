//go:build unix

package cmd

import (
	"time"

	"golang.org/x/sys/unix"
)

// stdinReadable waits until stdin has data or the timeout expires. The monitor
// polls stdin instead of reading it from a goroutine so that an external
// command can own stdin while it runs.
func stdinReadable(fd int, timeout time.Duration) bool {
	ms := int(timeout / time.Millisecond)
	if ms < 0 {
		ms = 0
	}
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	for {
		n, err := unix.Poll(fds, ms)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			// Polling is not supported here; let the caller do a blocking
			// read instead of spinning.
			return true
		}
		return n > 0
	}
}
