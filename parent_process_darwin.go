//go:build darwin
// +build darwin

package main

import (
	"syscall"
	"time"
)

func parentProcessDone(pid int) <-chan struct{} {
	if pid <= 1 {
		return nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			if !processExists(pid) {
				return
			}
		}
	}()

	return done
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
