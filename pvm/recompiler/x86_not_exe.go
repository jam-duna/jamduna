//go:build !(linux && amd64)
// +build !linux !amd64

package recompiler

import (
	"fmt"
	"runtime"
	"time"
)

// ExecuteX86 is a no-op stub on non-linux/amd64 platforms.
// Return codes reserved so far in your codebase:
//
//	 0  = success
//	-1  = SIGSEGV
//	-2  = mmap failed
//	-3  = mprotect failed
const errCodeNotSupported = -4

func ExecuteX86(code []byte, regBuf []byte) (ret int, usec int64, err error) {
	_ = code
	_ = regBuf
	start := time.Now()
	defer func() { usec = time.Since(start).Microseconds() }()

	return errCodeNotSupported, usec, fmt.Errorf(
		"x86 native execution is not supported on this platform (GOOS=%s, GOARCH=%s). "+
			"Use the Unicorn path on non-linux/amd64.",
		runtime.GOOS, runtime.GOARCH,
	)
}
