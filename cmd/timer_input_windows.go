//go:build windows

package cmd

import (
	"time"

	"golang.org/x/sys/windows"
)

// stdinReadable waits until stdin has data or the timeout expires. It is only
// used when stdin is a console, because a console input handle is signaled
// when input records are ready.
func stdinReadable(fd int, timeout time.Duration) bool {
	handle := windows.Handle(fd)
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		time.Sleep(timeout)
		return true
	}
	ms := uint32(timeout / time.Millisecond)
	if timeout < 0 {
		ms = 0
	}
	event, err := windows.WaitForSingleObject(handle, ms)
	if err != nil {
		return true
	}
	return event == windows.WAIT_OBJECT_0
}
