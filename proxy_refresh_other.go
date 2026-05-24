//go:build !darwin
// +build !darwin

package main

func startProxyRefreshLoop(host string, port string) func() {
	return func() {}
}
