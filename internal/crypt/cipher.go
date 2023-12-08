package crypt

import (
	"bytes"
	"context"
	"crypto/aes"
	gocipher "crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"sync"

	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/crypto/scrypt"
)

const (
	nameCipherBlockSize = aes.BlockSize
	fileMagic           = "TELDRIVE\x00\x00"
	fileMagicSize       = len(fileMagic)
	fileNonceSize       = 24
	fileHeaderSize      = fileMagicSize + fileNonceSize
	blockHeaderSize     = secretbox.Overhead
	blockDataSize       = 64 * 1024
	blockSize           = blockHeaderSize + blockDataSize
)

var (
	ErrorEncryptedFileTooShort  = errors.New("file is too short to be encrypted")
	ErrorEncryptedFileBadHeader = errors.New("file has truncated block header")
	ErrorEncryptedBadMagic      = errors.New("not an encrypted file - bad magic string")
	ErrorFileClosed             = errors.New("file already closed")
	ErrorBadSeek                = errors.New("Seek beyond end of file")
)

var (
	fileMagicBytes = []byte(fileMagic)
)

type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

type OpenRangeSeek func(ctx context.Context, offset, limit int64) (io.ReadCloser, error)

func readFill(r io.Reader, buf []byte) (n int, err error) {
	var nn int
	for n < len(buf) && err == nil {
		nn, err = r.Read(buf[n:])
		n += nn
	}
	return n, err
}

type Cipher struct {
	dataKey    [32]byte
	nameKey    [32]byte
	nameTweak  [nameCipherBlockSize]byte
	block      gocipher.Block
	buffers    sync.Pool
	cryptoRand io.Reader
}

func NewCipher(password, salt string) (*Cipher, error) {
	c := &Cipher{
		cryptoRand: rand.Reader,
	}
	c.buffers.New = func() interface{} {
		return new([blockSize]byte)
	}
	err := c.Key(password, salt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Cipher) Key(password, salt string) (err error) {
	const keySize = len(c.dataKey) + len(c.nameKey) + len(c.nameTweak)
	saltBytes := []byte(salt)
	key, err := scrypt.Key([]byte(password), saltBytes, 16384, 8, 1, keySize)
	if err != nil {
		return err
	}

	copy(c.dataKey[:], key)
	copy(c.nameKey[:], key[len(c.dataKey):])
	copy(c.nameTweak[:], key[len(c.dataKey)+len(c.nameKey):])

	c.block, err = aes.NewCipher(c.nameKey[:])
	return err
}

func (c *Cipher) getBlock() *[blockSize]byte {
	return c.buffers.Get().(*[blockSize]byte)
}

func (c *Cipher) putBlock(buf *[blockSize]byte) {
	c.buffers.Put(buf)
}

type nonce [fileNonceSize]byte

func (n *nonce) pointer() *[fileNonceSize]byte {
	return (*[fileNonceSize]byte)(n)
}

func (n *nonce) fromReader(in io.Reader) error {
	read, err := readFill(in, (*n)[:])
	if read != fileNonceSize {
		return fmt.Errorf("short read of nonce: %w", err)
	}
	return nil
}

func (n *nonce) fromBuf(buf []byte) error {
	read := copy((*n)[:], buf)

	if read != fileNonceSize {
		return errors.New("buffer to short to read nonce")
	}
	return nil
}

func (n *nonce) carry(i int) {
	for ; i < len(*n); i++ {
		digit := (*n)[i]
		newDigit := digit + 1
		(*n)[i] = newDigit
		if newDigit >= digit {
			// exit if no carry
			break
		}
	}
}

func (n *nonce) increment() {
	n.carry(0)
}

func (n *nonce) add(x uint64) {
	carry := uint16(0)
	for i := 0; i < 8; i++ {
		digit := (*n)[i]
		xDigit := byte(x)
		x >>= 8
		carry += uint16(digit) + uint16(xDigit)
		(*n)[i] = byte(carry)
		carry >>= 8
	}
	if carry != 0 {
		n.carry(8)
	}
}

type encrypter struct {
	mu       sync.Mutex
	in       io.Reader
	c        *Cipher
	nonce    nonce
	buf      *[blockSize]byte
	readBuf  *[blockSize]byte
	bufIndex int
	bufSize  int
	err      error
}

func (c *Cipher) newEncrypter(in io.Reader, nonce *nonce) (*encrypter, error) {
	fh := &encrypter{
		in:      in,
		c:       c,
		buf:     c.getBlock(),
		readBuf: c.getBlock(),
		bufSize: fileHeaderSize,
	}

	if nonce != nil {
		fh.nonce = *nonce
	} else {
		err := fh.nonce.fromReader(c.cryptoRand)
		if err != nil {
			return nil, err
		}
	}

	copy((*fh.buf)[:], fileMagicBytes)

	copy((*fh.buf)[fileMagicSize:], fh.nonce[:])
	return fh, nil
}

func (fh *encrypter) Read(p []byte) (n int, err error) {
	fh.mu.Lock()
	defer fh.mu.Unlock()

	if fh.err != nil {
		return 0, fh.err
	}
	if fh.bufIndex >= fh.bufSize {

		readBuf := (*fh.readBuf)[:blockDataSize]
		n, err = readFill(fh.in, readBuf)
		if n == 0 {
			return fh.finish(err)
		}

		secretbox.Seal((*fh.buf)[:0], readBuf[:n], fh.nonce.pointer(), &fh.c.dataKey)
		fh.bufIndex = 0
		fh.bufSize = blockHeaderSize + n
		fh.nonce.increment()
	}
	n = copy(p, (*fh.buf)[fh.bufIndex:fh.bufSize])
	fh.bufIndex += n
	return n, nil
}

func (fh *encrypter) finish(err error) (int, error) {
	if fh.err != nil {
		return 0, fh.err
	}
	fh.err = err
	fh.c.putBlock(fh.buf)
	fh.buf = nil
	fh.c.putBlock(fh.readBuf)
	fh.readBuf = nil
	return 0, err
}

func (fh *encrypter) Close() error {
	return nil
}

func (c *Cipher) EncryptData(in io.Reader) (io.ReadCloser, error) {
	return c.newEncrypter(in, nil)
}

type decrypter struct {
	mu           sync.Mutex
	rc           io.ReadCloser
	nonce        nonce
	initialNonce nonce
	c            *Cipher
	buf          *[blockSize]byte
	readBuf      *[blockSize]byte
	bufIndex     int
	bufSize      int
	err          error
	limit        int64
	open         OpenRangeSeek
}

func (c *Cipher) newDecrypter(rc io.ReadCloser) (*decrypter, error) {
	fh := &decrypter{
		rc:      rc,
		c:       c,
		buf:     c.getBlock(),
		readBuf: c.getBlock(),
		limit:   -1,
	}

	readBuf := (*fh.readBuf)[:fileHeaderSize]
	n, err := readFill(fh.rc, readBuf)
	if n < fileHeaderSize && err == io.EOF {

		return nil, fh.finishAndClose(ErrorEncryptedFileTooShort)
	} else if err != io.EOF && err != nil {
		return nil, fh.finishAndClose(err)
	}

	if !bytes.Equal(readBuf[:fileMagicSize], fileMagicBytes) {
		return nil, fh.finishAndClose(ErrorEncryptedBadMagic)
	}

	err = fh.nonce.fromBuf(readBuf[fileMagicSize:])
	if err != nil {
		return nil, err
	}
	fh.initialNonce = fh.nonce
	return fh, nil
}

func (c *Cipher) newDecrypterSeek(ctx context.Context, open OpenRangeSeek, offset, limit int64) (fh *decrypter, err error) {
	var rc io.ReadCloser
	doRangeSeek := false
	setLimit := false

	if offset == 0 && limit < 0 {

		rc, err = open(ctx, 0, -1)
	} else if offset == 0 {

		_, underlyingLimit, _, _ := calculateUnderlying(offset, limit)
		rc, err = open(ctx, 0, int64(fileHeaderSize)+underlyingLimit)
		setLimit = true
	} else {

		rc, err = open(ctx, 0, int64(fileHeaderSize))
		doRangeSeek = true
	}
	if err != nil {
		return nil, err
	}

	fh, err = c.newDecrypter(rc)
	if err != nil {
		return nil, err
	}
	fh.open = open
	if doRangeSeek {
		_, err = fh.RangeSeek(ctx, offset, io.SeekStart, limit)
		if err != nil {
			_ = fh.Close()
			return nil, err
		}
	}
	if setLimit {
		fh.limit = limit
	}
	return fh, nil
}

func (fh *decrypter) fillBuffer() (err error) {

	readBuf := fh.readBuf
	n, err := readFill(fh.rc, (*readBuf)[:])
	if n == 0 {
		return err
	}

	if n <= blockHeaderSize {
		if err != nil && err != io.EOF {
			return err
		}
		return ErrorEncryptedFileBadHeader
	}

	_, ok := secretbox.Open((*fh.buf)[:0], (*readBuf)[:n], fh.nonce.pointer(), &fh.c.dataKey)
	if !ok {
		if err != nil && err != io.EOF {
			return err
		}

		for i := range (*fh.buf)[:n] {
			(*fh.buf)[i] = 0
		}
	}
	fh.bufIndex = 0
	fh.bufSize = n - blockHeaderSize
	fh.nonce.increment()
	return nil
}

func (fh *decrypter) Read(p []byte) (n int, err error) {
	fh.mu.Lock()
	defer fh.mu.Unlock()

	if fh.err != nil {
		return 0, fh.err
	}
	if fh.bufIndex >= fh.bufSize {
		err = fh.fillBuffer()
		if err != nil {
			return 0, fh.finish(err)
		}
	}
	toCopy := fh.bufSize - fh.bufIndex
	if fh.limit >= 0 && fh.limit < int64(toCopy) {
		toCopy = int(fh.limit)
	}
	n = copy(p, (*fh.buf)[fh.bufIndex:fh.bufIndex+toCopy])
	fh.bufIndex += n
	if fh.limit >= 0 {
		fh.limit -= int64(n)
		if fh.limit == 0 {
			return n, fh.finish(io.EOF)
		}
	}
	return n, nil
}

func calculateUnderlying(offset, limit int64) (underlyingOffset, underlyingLimit, discard, blocks int64) {

	blocks, discard = offset/blockDataSize, offset%blockDataSize

	underlyingOffset = int64(fileHeaderSize) + blocks*(blockHeaderSize+blockDataSize)

	underlyingLimit = int64(-1)
	if limit >= 0 {

		bytesToRead := limit - (blockDataSize - discard)

		blocksToRead := int64(1)

		if bytesToRead > 0 {

			extraBlocksToRead, endBytes := bytesToRead/blockDataSize, bytesToRead%blockDataSize
			if endBytes != 0 {

				extraBlocksToRead++
			}
			blocksToRead += extraBlocksToRead
		}

		underlyingLimit = blocksToRead * (blockHeaderSize + blockDataSize)
	}
	return
}

func (fh *decrypter) RangeSeek(ctx context.Context, offset int64, whence int, limit int64) (int64, error) {
	fh.mu.Lock()
	defer fh.mu.Unlock()

	if fh.open == nil {
		return 0, fh.finish(errors.New("can't seek - not initialised with newDecrypterSeek"))
	}
	if whence != io.SeekStart {
		return 0, fh.finish(errors.New("can only seek from the start"))
	}

	if fh.err == io.EOF {
		fh.unFinish()
	} else if fh.err != nil {
		return 0, fh.err
	}

	underlyingOffset, underlyingLimit, discard, blocks := calculateUnderlying(offset, limit)

	fh.nonce = fh.initialNonce
	fh.nonce.add(uint64(blocks))

	rc, err := fh.open(ctx, underlyingOffset, underlyingLimit)
	if err != nil {
		return 0, fh.finish(fmt.Errorf("couldn't reopen file with offset and limit: %w", err))
	}

	fh.rc = rc

	err = fh.fillBuffer()
	if err != nil {
		return 0, fh.finish(err)
	}

	if int(discard) > fh.bufSize {
		return 0, fh.finish(ErrorBadSeek)
	}
	fh.bufIndex = int(discard)

	fh.limit = limit

	return offset, nil
}

func (fh *decrypter) Seek(offset int64, whence int) (int64, error) {
	return fh.RangeSeek(context.TODO(), offset, whence, -1)
}

func (fh *decrypter) finish(err error) error {
	if fh.err != nil {
		return fh.err
	}
	fh.err = err
	fh.c.putBlock(fh.buf)
	fh.buf = nil
	fh.c.putBlock(fh.readBuf)
	fh.readBuf = nil
	return err
}

func (fh *decrypter) unFinish() {

	fh.err = nil

	fh.buf = fh.c.getBlock()
	fh.readBuf = fh.c.getBlock()

	fh.bufIndex = 0
	fh.bufSize = 0
}

func (fh *decrypter) Close() error {
	fh.mu.Lock()
	defer fh.mu.Unlock()

	if fh.err == ErrorFileClosed {
		return fh.err
	}

	if fh.err == nil {
		_ = fh.finish(io.EOF)
	}

	fh.err = ErrorFileClosed
	if fh.rc == nil {
		return nil
	}
	return fh.rc.Close()
}

func (fh *decrypter) finishAndClose(err error) error {
	_ = fh.finish(err)
	_ = fh.Close()
	return err
}

func (c *Cipher) DecryptData(rc io.ReadCloser) (io.ReadCloser, error) {
	out, err := c.newDecrypter(rc)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Cipher) DecryptDataSeek(ctx context.Context, open OpenRangeSeek, offset, limit int64) (ReadSeekCloser, error) {
	out, err := c.newDecrypterSeek(ctx, open, offset, limit)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func EncryptedSize(size int64) int64 {
	blocks, residue := size/blockDataSize, size%blockDataSize
	encryptedSize := int64(fileHeaderSize) + blocks*(blockHeaderSize+blockDataSize)
	if residue != 0 {
		encryptedSize += blockHeaderSize + residue
	}
	return encryptedSize
}

func DecryptedSize(size int64) (int64, error) {
	size -= int64(fileHeaderSize)
	if size < 0 {
		return 0, ErrorEncryptedFileTooShort
	}
	blocks, residue := size/blockSize, size%blockSize
	decryptedSize := blocks * blockDataSize
	if residue != 0 {
		residue -= blockHeaderSize
		if residue <= 0 {
			return 0, ErrorEncryptedFileBadHeader
		}
	}
	decryptedSize += residue
	return decryptedSize, nil
}
