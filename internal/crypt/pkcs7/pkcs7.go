package pkcs7

import "errors"

var (
	ErrorPaddingNotFound      = errors.New("bad PKCS#7 padding - not padded")
	ErrorPaddingNotAMultiple  = errors.New("bad PKCS#7 padding - not a multiple of blocksize")
	ErrorPaddingTooLong       = errors.New("bad PKCS#7 padding - too long")
	ErrorPaddingTooShort      = errors.New("bad PKCS#7 padding - too short")
	ErrorPaddingNotAllTheSame = errors.New("bad PKCS#7 padding - not all the same")
)

func Pad(n int, buf []byte) []byte {
	if n <= 1 || n >= 256 {
		panic("bad multiple")
	}
	length := len(buf)
	padding := n - (length % n)
	for i := 0; i < padding; i++ {
		buf = append(buf, byte(padding))
	}
	if (len(buf) % n) != 0 {
		panic("padding failed")
	}
	return buf
}

func Unpad(n int, buf []byte) ([]byte, error) {
	if n <= 1 || n >= 256 {
		panic("bad multiple")
	}
	length := len(buf)
	if length == 0 {
		return nil, ErrorPaddingNotFound
	}
	if (length % n) != 0 {
		return nil, ErrorPaddingNotAMultiple
	}
	padding := int(buf[length-1])
	if padding > n {
		return nil, ErrorPaddingTooLong
	}
	if padding == 0 {
		return nil, ErrorPaddingTooShort
	}
	for i := 0; i < padding; i++ {
		if buf[length-1-i] != byte(padding) {
			return nil, ErrorPaddingNotAllTheSame
		}
	}
	return buf[:length-padding], nil
}
