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
	if len(os.Args) > 2 {
		return fmt.Errorf("usage: just update-bin [VERSION]")
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	var version string
	if len(os.Args) == 2 && os.Args[1] != "" {
		version = strings.TrimPrefix(os.Args[1], "v")
	} else {
		latest, err := latestVersion(client, "https://api.github.com/repos/tgdrive/teldrive/releases/latest")
		if err != nil {
			return err
		}
		version = latest
	}
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
	if strings.HasPrefix(version, "0.") || strings.HasPrefix(version, "1.") {
		return fmt.Errorf("release %s predates the v2 binary package; publish 2.0.0 first", version)
	}
	fmt.Printf("Updating binary package to %s\n", version)
	hashes := map[string]string{}
	for _, arch := range []string{"amd64", "arm64"} {
		url := fmt.Sprintf("https://github.com/tgdrive/teldrive/releases/download/%s/teldrive-%s-linux-%s.tar.gz", version, version, arch)
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
	for _, args := range [][]string{{"add", "--", path}, {"commit", "-m", "chore(nix): pin teldrive-bin " + version, "--", path}} {
		cmd := exec.Command("git", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

func latestVersion(client *http.Client, url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "teldrive-bin-updater")
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("detect latest release: %s", response.Status)
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(response.Body).Decode(&release); err != nil {
		return "", err
	}
	if release.Draft || release.Prerelease || release.Tag == "" {
		return "", fmt.Errorf("latest release is not a published stable release")
	}
	return strings.TrimPrefix(release.Tag, "v"), nil
}
