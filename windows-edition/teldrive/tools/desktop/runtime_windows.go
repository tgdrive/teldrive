package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func runtimeInstalled() bool {
	for _, name := range []string{"vcruntime140.dll", "msvcp140.dll"} {
		if _, e := os.Stat(filepath.Join(os.Getenv("SystemRoot"), "System32", name)); e != nil {
			return false
		}
	}
	return true
}
func (m *manager) installRuntime() error {
	// Download directly from Microsoft only when requested; the proprietary runtime is not redistributed in the FLOSS bundle.
	client := &http.Client{Timeout: 2 * time.Minute}
	response, e := client.Get("https://aka.ms/vs/17/release/vc_redist.x64.exe")
	if e != nil {
		return e
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("microsoft no pudo entregar el instalador")
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if e != nil {
		return e
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != "cc0ff0eb1dc3f5188ae6300faef32bf5beeba4bdd6e8e445a9184072096b713b" {
		return errors.New("microsoft cambió el instalador. Actualiza el programa o usa el enlace oficial de la interfaz")
	}
	p := filepath.Join(m.dir, "vc_redist.x64.exe")
	if e = os.WriteFile(p, data, 0600); e != nil {
		return e
	}
	c := exec.Command(p, "/norestart")
	hidden(c)
	if e = c.Start(); e != nil {
		return e
	}
	go c.Wait()
	return nil
}
