// Package tdhash implements the Teldrive hashing algorithm.
// Files are split into 16MB blocks (fixed size).
// Each block is hashed with BLAKE3.
// Block hashes are concatenated and hashed together to produce the final tree hash.
package tdhash

import (
	"encoding/hex"
	"hash"

	"github.com/zeebo/blake3"
)

const (
	// BlockSize is the fixed block size for tree hashing (16MB).
	BlockSize = 16 * 1024 * 1024
	// Size is the expected hex-encoded string length (64 chars for 32-byte blake3 hash).
	// Note: Sum() returns 32 raw bytes, rclone will hex-encode for display.
	Size = 64
	// rawSize is the actual number of bytes returned by Sum().
	rawSize = 32
)

type digest struct {
	chunkHash   hash.Hash // Hash for current block
	totalHash   hash.Hash // Tree hash (hash of block hashes)
	n           int64     // bytes written into current block
	sumCalled   bool
	writtenMore bool
}

// New creates a new tree hash (uses BLAKE3 with fixed 16MB blocks)
func New() hash.Hash {
	d := &digest{}
	d.Reset()
	return d
}

// Reset resets the hash to its initial state.
func (d *digest) Reset() {
	d.n = 0
	d.sumCalled = false
	d.writtenMore = false

	// Always use BLAKE3
	d.chunkHash = blake3.New()
	d.totalHash = blake3.New()
}

// writeChunkHash writes the current chunk hash into the total hash
func (d *digest) writeChunkHash() {
	chunkHashBytes := d.chunkHash.Sum(nil)
	d.totalHash.Write(chunkHashBytes)
	// Reset chunk hasher for next chunk
	d.n = 0
	d.chunkHash.Reset()
}

// Write adds more data to the running hash.
// It processes data in chunks and accumulates chunk hashes.
func (d *digest) Write(p []byte) (n int, err error) {
	n = len(p)
	d.writtenMore = true

	for len(p) > 0 {
		// Calculate how much we can write to current block
		remainingInBlock := BlockSize - d.n
		toWrite := min(int64(len(p)), remainingInBlock)

		// Write to current block hash
		d.chunkHash.Write(p[:toWrite])
		d.n += toWrite
		p = p[toWrite:]

		// If block is full, finalize it and start new block
		if d.n >= BlockSize {
			d.writeChunkHash()
		}
	}

	return n, nil
}

// Sum returns the current hash as raw bytes (32 bytes for BLAKE3).
// Note: rclone's hash system will hex-encode this for display/comparison.
func (d *digest) Sum(b []byte) []byte {
	if d.sumCalled && d.writtenMore {
		// If Sum was already called and we wrote more data, we need to
		// finalize the last chunk if it has data
		if d.n > 0 {
			d.writeChunkHash()
		}
	}

	// Finalize last chunk if it has data
	if d.n > 0 {
		d.writeChunkHash()
	}

	d.sumCalled = true
	d.writtenMore = false

	// Return raw tree hash bytes (not hex-encoded)
	treeHashBytes := d.totalHash.Sum(nil)
	return append(b, treeHashBytes...)
}

// Size returns the number of bytes Sum will return (32 for BLAKE3).
func (d *digest) Size() int {
	return rawSize
}

// BlockSize returns the hash's underlying block size.
func (d *digest) BlockSize() int {
	return BlockSize
}

// SumString computes the tree hash of the entire file and returns it as a hex string
func SumString(data []byte) string {
	d := New()
	d.Write(data)
	return hex.EncodeToString(d.Sum(nil))
}
