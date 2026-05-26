package main

import (
	"crypto/tls"
	"os"
	"path/filepath"

	"github.com/elazarl/goproxy"
)

const caCertName = "firefly-go-proxy-ca.crt"

func setupCertificate(installSystemCA bool) (*tls.Certificate, error) {
	if _, err := os.Stat(caCertName); os.IsNotExist(err) {
		if err := os.WriteFile(caCertName, goproxy.GoproxyCa.Certificate[0], 0644); err != nil {
			return nil, err
		}
	}

	if !installSystemCA {
		return &goproxy.GoproxyCa, nil
	}

	absPath, err := filepath.Abs(caCertName)
	if err != nil {
		return nil, err
	}
	if err := installCA(absPath); err != nil {
		return nil, err
	}

	return &goproxy.GoproxyCa, nil
}
