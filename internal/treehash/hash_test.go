package treehash

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestOriginalTelDriveHashVectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		data       []byte
		blocksHex  string
		treeHex    string
		blockCount int
	}{
		{
			name:       "small",
			data:       []byte("abc"),
			blocksHex:  "6437b3ac38465133ffb63b75273a8db548c558465d79db03fd359c6cd5bd9d85",
			treeHex:    "dc2f738d17b8a7ec03efdd0a95e8d3924b8e05965040ab6dcafd992994012bd4",
			blockCount: 0,
		},
		{
			name:       "crosses block boundary",
			data:       bytes.Repeat([]byte{0x5a}, BlockSize+3),
			blocksHex:  "5fc8984b71b8697e2b1d2042f986bd295506458b27c45783286aead635122af110ec15e899c3e59db61da2026868a6d023c87d7e6908579eb89d985a2c3c8636",
			treeHex:    "abe2ab5b86203c299e1833d1fef91a84ed28749e8b5fc194644f78ffa7fa0ec9",
			blockCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewBlockHasher()
			if _, err := h.Write(tt.data); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			first := h.Sum()
			second := h.Sum()
			if !bytes.Equal(first, second) {
				t.Fatal("Sum must be idempotent")
			}
			if got := hex.EncodeToString(first); got != tt.blocksHex {
				t.Fatalf("block hashes = %s, want %s", got, tt.blocksHex)
			}
			if got := SumToHex(ComputeTreeHash(first)); got != tt.treeHex {
				t.Fatalf("tree hash = %s, want %s", got, tt.treeHex)
			}
			if got := h.BlockCount(); got != tt.blockCount {
				t.Fatalf("BlockCount() = %d, want %d", got, tt.blockCount)
			}
		})
	}
}

func TestReset(t *testing.T) {
	t.Parallel()
	h := NewBlockHasher()
	_, _ = h.Write([]byte("first"))
	_ = h.Sum()
	h.Reset()
	_, _ = h.Write([]byte("abc"))
	if got := SumToHex(h.Sum()); got != "6437b3ac38465133ffb63b75273a8db548c558465d79db03fd359c6cd5bd9d85" {
		t.Fatalf("hash after reset = %s", got)
	}
}
