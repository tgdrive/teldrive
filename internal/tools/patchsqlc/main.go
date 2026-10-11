package main

import (
	"bytes"
	"fmt"
	"os"
)

const generatedDBPath = "internal/db/sqlcgen/db.go"

type replacement struct {
	from []byte
	to   []byte
}

var replacements = []replacement{
	{from: []byte("return &Queries{db: db}"), to: []byte("return &Queries{db: wrapDBTX(db)}")},
	{from: []byte("\t\tdb: tx,"), to: []byte("\t\tdb: wrapDBTX(tx),")},
}

func main() {
	if err := patchFile(generatedDBPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func patchFile(path string) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	patched := append([]byte(nil), contents...)
	for _, replacement := range replacements {
		sourceCount := bytes.Count(patched, replacement.from)
		targetCount := bytes.Count(patched, replacement.to)
		switch {
		case sourceCount == 1:
			patched = bytes.Replace(patched, replacement.from, replacement.to, 1)
		case sourceCount == 0 && targetCount == 1:
			// Already patched; keep the tool safe to run repeatedly.
		default:
			return fmt.Errorf(
				"patch %s: expected exactly one generated source or patched target, got source=%d target=%d",
				path,
				sourceCount,
				targetCount,
			)
		}
	}

	if bytes.Equal(contents, patched) {
		return nil
	}
	if err := os.WriteFile(path, patched, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
