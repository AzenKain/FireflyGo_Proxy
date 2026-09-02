//go:build linux
// +build linux

package main

func setProxy(enable bool, host string, port string) error {
	if enable {
		ENV_CONFIG = []string{
			"HTTP_PROXY=http://" + host + ":" + port,
			"http_proxy=http://" + host + ":" + port,
			"HTTPS_PROXY=http://" + host + ":" + port,
			"https_proxy=http://" + host + ":" + port,
		}
	} else {
		ENV_CONFIG = nil
	}

	return nil
}
