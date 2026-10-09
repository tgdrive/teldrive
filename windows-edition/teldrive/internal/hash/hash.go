package hash

import (
	"encoding/hex"

	"github.com/zeebo/blake3"
)

// BlockSize is the fixed block size for tree hashing (16MB)
const BlockSize = 16 * 1024 * 1024

// Type represents the hash algorithm type
type Type string

const (
	// TypeBlake3 is the only supported hash algorithm (fastest)
	TypeBlake3 Type = "blake3"
)

// BlockHasher processes data in fixed-size blocks and accumulates block hashes
type BlockHasher struct {
	blockSize    int64
	currentHash  *blake3.Hasher
	blockHashes  [][]byte
	bytesInBlock int64
}

// NewBlockHasher creates a new BlockHasher (always BLAKE3)
func NewBlockHasher() *BlockHasher {
	return &BlockHasher{
		blockSize: BlockSize,
	}
}

// Write implements io.Writer - processes data in BlockSize chunks
func (h *BlockHasher) Write(p []byte) (n int, err error) {
	n = len(p)

	for len(p) > 0 {
		remaining := h.blockSize - h.bytesInBlock
		toWrite := min(int64(len(p)), remaining)

		// Initialize hash if this is a new block
		if h.bytesInBlock == 0 {
			h.currentHash = blake3.New()
		}

		h.currentHash.Write(p[:toWrite])
		h.bytesInBlock += toWrite
		p = p[toWrite:]

		// Block is complete
		if h.bytesInBlock >= h.blockSize {
			h.blockHashes = append(h.blockHashes, h.currentHash.Sum(nil))
			h.bytesInBlock = 0
		}
	}

	return n, nil
}

// Sum returns concatenated block hashes
func (h *BlockHasher) Sum() []byte {
	// Handle partial block at end
	if h.bytesInBlock > 0 {
		h.blockHashes = append(h.blockHashes, h.currentHash.Sum(nil))
		h.bytesInBlock = 0
	}

	// Concatenate all block hashes
	var result []byte
	for _, bh := range h.blockHashes {
		result = append(result, bh...)
	}
	return result
}

// GetBlockCount returns the number of complete blocks processed
func (h *BlockHasher) GetBlockCount() int {
	return len(h.blockHashes)
}

// Reset resets the hasher for a new stream
func (h *BlockHasher) Reset() {
	h.blockHashes = nil
	h.bytesInBlock = 0
}

// ComputeTreeHash computes the final tree hash from concatenated block hashes
func ComputeTreeHash(concatenatedBlockHashes []byte) []byte {
	h := blake3.New()
	h.Write(concatenatedBlockHashes)
	return h.Sum(nil)
}

// SumToHex converts bytes to hex string
func SumToHex(data []byte) string {
	return hex.EncodeToString(data)
}
