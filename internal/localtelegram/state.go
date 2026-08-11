package localtelegram

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const stateVersion = 1

type persistedState struct {
	Version        int                       `json:"version"`
	NextChannelID  int64                     `json:"nextChannelId"`
	NextDocumentID int64                     `json:"nextDocumentId"`
	NextMessageID  int                       `json:"nextMessageId"`
	Channels       map[string]channelRecord  `json:"channels"`
	Documents      map[string]documentRecord `json:"documents"`
	Messages       map[string]messageRecord  `json:"messages"`
}

type channelRecord struct {
	ID         int64  `json:"id"`
	AccessHash int64  `json:"accessHash"`
	Title      string `json:"title"`
	CreatedAt  int    `json:"createdAt"`
}

type documentRecord struct {
	ID            int64  `json:"id"`
	AccessHash    int64  `json:"accessHash"`
	FileReference []byte `json:"fileReference"`
	MimeType      string `json:"mimeType"`
	Size          int64  `json:"size"`
	DCID          int    `json:"dcId"`
	FileName      string `json:"fileName"`
	CreatedAt     int    `json:"createdAt"`
}

type messageRecord struct {
	ChannelID  int64 `json:"channelId"`
	ID         int   `json:"id"`
	DocumentID int64 `json:"documentId"`
	CreatedAt  int   `json:"createdAt"`
}

func newState() persistedState {
	return persistedState{
		Version:        stateVersion,
		NextChannelID:  1000,
		NextDocumentID: 10000,
		NextMessageID:  1,
		Channels:       make(map[string]channelRecord),
		Documents:      make(map[string]documentRecord),
		Messages:       make(map[string]messageRecord),
	}
}

func loadState(path string) (persistedState, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return newState(), nil
	}
	if err != nil {
		return persistedState{}, fmt.Errorf("read local Telegram state: %w", err)
	}
	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return persistedState{}, fmt.Errorf("decode local Telegram state: %w", err)
	}
	if state.Version != stateVersion {
		return persistedState{}, fmt.Errorf("unsupported local Telegram state version %d", state.Version)
	}
	if state.Channels == nil {
		state.Channels = make(map[string]channelRecord)
	}
	if state.Documents == nil {
		state.Documents = make(map[string]documentRecord)
	}
	if state.Messages == nil {
		state.Messages = make(map[string]messageRecord)
	}
	if state.NextChannelID <= 0 {
		state.NextChannelID = 1000
	}
	if state.NextDocumentID <= 0 {
		state.NextDocumentID = 10000
	}
	if state.NextMessageID <= 0 {
		state.NextMessageID = 1
	}
	return state, nil
}

func saveState(path string, state persistedState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode local Telegram state: %w", err)
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(path), ".state-*.json")
	if err != nil {
		return fmt.Errorf("create local Telegram state temp file: %w", err)
	}
	tempName := temp.Name()
	cleanup := true
	defer func() {
		_ = temp.Close()
		if cleanup {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod local Telegram state temp file: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("write local Telegram state temp file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync local Telegram state temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close local Telegram state temp file: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace local Telegram state: %w", err)
	}
	cleanup = false
	return nil
}

func channelKey(id int64) string  { return strconv.FormatInt(id, 10) }
func documentKey(id int64) string { return strconv.FormatInt(id, 10) }
func messageKey(channelID int64, messageID int) string {
	return strconv.FormatInt(channelID, 10) + ":" + strconv.Itoa(messageID)
}
