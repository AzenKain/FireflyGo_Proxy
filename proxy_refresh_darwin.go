//go:build darwin
// +build darwin

package main

import (
	"sync"
	"time"

	zlog "github.com/rs/zerolog/log"
)

func startProxyRefreshLoop(host string, port string) func() {
	done := make(chan struct{})
	stopped := make(chan struct{})
	var once sync.Once

	go func() {
		defer close(stopped)

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := setProxy(true, host, port); err != nil {
					zlog.Error().Err(err).Msg("Failed to refresh macOS proxy services")
				}
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(done)
			<-stopped
		})
	}
}
