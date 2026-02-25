package main

import (
	"fmt"
	"os"
	"strings"
)

type operationPatch struct {
	pathText   string
	methodText string
	mediaType  string
	fieldKey   string
	fieldValue string
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "postprocess-openapi: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	const filePath = "../openapi/openapi.yaml"

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	updated := string(content)
	patches := []operationPatch{
		{
			pathText:   "/auth/login:",
			methodText: "post:",
			fieldKey:   "x-ogen-raw-response",
			fieldValue: "true",
		},
		{
			pathText:   "/auth/logout:",
			methodText: "post:",
			fieldKey:   "x-ogen-raw-response",
			fieldValue: "true",
		},
		{
			pathText:   "/auth/refresh:",
			methodText: "post:",
			fieldKey:   "x-ogen-raw-response",
			fieldValue: "true",
		},
		{
			pathText:   "/files/{id}/content:",
			methodText: "get:",
			mediaType:  "application/octet-stream:",
			fieldKey:   "x-ogen-raw-response",
			fieldValue: "true",
		},
		{
			pathText:   "/events/stream:",
			methodText: "get:",
			mediaType:  "text/event-stream:",
			fieldKey:   "x-ogen-raw-response",
			fieldValue: "true",
		},
		{
			pathText:   "/auth/attempts/{id}/events:",
			methodText: "get:",
			mediaType:  "text/event-stream:",
			fieldKey:   "x-ogen-raw-response",
			fieldValue: "true",
		},
		{
			pathText:   "/shares/{id}/files/{fileId}/content:",
			methodText: "get:",
			mediaType:  "application/octet-stream:",
			fieldKey:   "x-ogen-raw-response",
			fieldValue: "true",
		},
	}

	for _, patch := range patches {
		updated, err = insertOperationField(updated, patch)
		if err != nil {
			return err
		}
	}

	if updated == string(content) {
		return nil
	}

	return os.WriteFile(filePath, []byte(updated), 0o644)
}

func insertOperationField(content string, patch operationPatch) (string, error) {
	lines := strings.Split(content, "\n")
	pathIndex := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == patch.pathText {
			pathIndex = i
			break
		}
	}
	if pathIndex == -1 {
		return "", fmt.Errorf("path block not found: %s", patch.pathText)
	}

	methodIndex := -1
	for i := pathIndex + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "  /") {
			break
		}
		if strings.TrimSpace(line) == patch.methodText {
			methodIndex = i
			break
		}
	}
	if methodIndex == -1 {
		return "", fmt.Errorf("method block not found for %s %s", patch.pathText, patch.methodText)
	}

	if patch.mediaType == "" {
		methodIndent := leadingWhitespace(lines[methodIndex])
		fieldLine := methodIndent + "  " + patch.fieldKey + ": " + patch.fieldValue
		methodEnd := len(lines)
		for i := methodIndex + 1; i < len(lines); i++ {
			line := lines[i]
			if strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, methodIndent+"  ") {
				methodEnd = i
				break
			}
			if strings.HasPrefix(line, "  /") {
				methodEnd = i
				break
			}
		}
		for i := methodIndex + 1; i < methodEnd; i++ {
			if lines[i] == fieldLine {
				return content, nil
			}
		}
		insertAt := methodIndex + 1
		updated := make([]string, 0, len(lines)+1)
		updated = append(updated, lines[:insertAt]...)
		updated = append(updated, fieldLine)
		updated = append(updated, lines[insertAt:]...)
		result := strings.Join(updated, "\n")
		if strings.HasSuffix(content, "\n") && !strings.HasSuffix(result, "\n") {
			result += "\n"
		}
		if !strings.HasSuffix(content, "\n") {
			result = strings.TrimSuffix(result, "\n")
		}
		return result, nil
	}

	methodEnd := len(lines)
	for i := methodIndex + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") {
			methodEnd = i
			break
		}
		if strings.HasPrefix(line, "  /") {
			methodEnd = i
			break
		}
	}

	inserted := false
	mediaMatches := 0
	for i := methodIndex + 1; i < methodEnd; i++ {
		if strings.TrimSpace(lines[i]) != patch.mediaType {
			continue
		}
		mediaMatches++
		mediaIndent := leadingWhitespace(lines[i])
		fieldLine := mediaIndent + "  " + patch.fieldKey + ": " + patch.fieldValue
		mediaEnd := methodEnd
		for j := i + 1; j < methodEnd; j++ {
			line := lines[j]
			if len(leadingWhitespace(line)) <= len(mediaIndent) && strings.TrimSpace(line) != "" {
				mediaEnd = j
				break
			}
		}
		exists := false
		for j := i + 1; j < mediaEnd; j++ {
			if lines[j] == fieldLine {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		insertAt := i + 1
		updated := make([]string, 0, len(lines)+1)
		updated = append(updated, lines[:insertAt]...)
		updated = append(updated, fieldLine)
		updated = append(updated, lines[insertAt:]...)
		lines = updated
		methodEnd++
		i++
		inserted = true
	}

	if mediaMatches == 0 {
		return "", fmt.Errorf("media type block not found for %s %s %s", patch.pathText, patch.methodText, patch.mediaType)
	}
	if !inserted {
		return content, nil
	}

	result := strings.Join(lines, "\n")
	if strings.HasSuffix(content, "\n") && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	if !strings.HasSuffix(content, "\n") {
		result = strings.TrimSuffix(result, "\n")
	}
	return result, nil
}

func leadingWhitespace(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	return line[:len(line)-len(trimmed)]
}
