package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml"
	"github.com/tgdrive/teldrive/internal/config"
	"gopkg.in/yaml.v3"
)

func main() {
	var (
		tomlPath = flag.String("toml", "config.sample.toml", "Path to TOML sample config")
		yamlPath = flag.String("yaml", "config.sample.yml", "Path to YAML sample config")
	)
	flag.Parse()

	defaults := config.DefaultServerConfigMap()

	if err := writeTOML(*tomlPath, defaults); err != nil {
		fail(err)
	}

	if err := writeYAML(*yamlPath, defaults); err != nil {
		fail(err)
	}
}

func writeTOML(path string, data map[string]any) error {
	content, err := toml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal toml: %w", err)
	}
	content = bytes.TrimLeft(content, "\n")
	return writeFile(path, content)
}

func writeYAML(path string, data map[string]any) error {
	content, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}
	return writeFile(path, content)
}

func writeFile(path string, content []byte) error {
	if filepath.Dir(path) != "." {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create parent dir: %w", err)
		}
	}

	if len(content) == 0 || content[len(content)-1] != '\n' {
		content = append(content, '\n')
	}

	return os.WriteFile(path, content, 0o644)
}

func fail(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
