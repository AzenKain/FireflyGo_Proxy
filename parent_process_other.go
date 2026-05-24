//go:build !darwin
// +build !darwin

package main

func parentProcessDone(pid int) <-chan struct{} {
	return nil
}
