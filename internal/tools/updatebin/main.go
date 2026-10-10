package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: just update-bin VERSION")
	}
	version := strings.TrimPrefix(os.Args[1], "v")
	if !regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(version) {
		return fmt.Errorf("invalid stable version")
	}
	status, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return err
	}
	if len(status) != 0 {
		return fmt.Errorf("commit existing changes before updating release pins")
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	hashes := map[string]string{}
	for _, arch := range []string{"amd64", "arm64"} {
		url := fmt.Sprintf("https://github.com/tgdrive/teldrive/releases/download/v%s/teldrive-v%s-linux-%s.tar.gz", version, version, arch)
		response, err := client.Get(url)
		if err != nil {
			return err
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return fmt.Errorf("download %s: %s", url, response.Status)
		}
		hash := sha256.New()
		_, err = io.Copy(hash, response.Body)
		response.Body.Close()
		if err != nil {
			return err
		}
		hashes[arch] = "sha256-" + base64.StdEncoding.EncodeToString(hash.Sum(nil))
	}
	data, err := json.MarshalIndent(struct {
		Version string            `json:"version"`
		Hashes  map[string]string `json:"hashes"`
	}{version, hashes}, "", "  ")
	if err != nil {
		return err
	}
	path := "nix/release.json"
	old, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if string(old) == string(data) {
		fmt.Println("Release pins already current")
		return nil
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	for _, args := range [][]string{{"add", "--", path}, {"commit", "-m", "chore(nix): pin teldrive-bin v" + version, "--", path}} {
		cmd := exec.Command("git", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}
