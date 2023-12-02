package md5

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
)

func FromBytes(data []byte) string {
	result := md5.Sum(data)
	return fmt.Sprintf("%x", result)
}

func FromString(str string) string {
	data := []byte(str)
	return FromBytes(data)
}

func FromFilePath(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	return FromReader(f)
}

func FromReader(src io.Reader) (string, error) {
	h := md5.New()
	if _, err := io.Copy(h, src); err != nil {
		return "", err
	}
	checksum := h.Sum(nil)
	return fmt.Sprintf("%x", checksum), nil
}
