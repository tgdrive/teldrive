//go:build ignore

package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	urlFlag := flag.String("url", "", "URL to download from")
	outputFlag := flag.String("output", "", "Output directory")
	flag.Parse()

	if *urlFlag == "" || *outputFlag == "" {
		flag.Usage()
		os.Exit(1)
	}

	if err := os.RemoveAll(*outputFlag); err != nil {
		fmt.Printf("Error removing directory: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(*outputFlag, 0755); err != nil {
		fmt.Printf("Error creating directory: %v\n", err)
		os.Exit(1)
	}

	resp, err := http.Get(*urlFlag)
	if err != nil {
		fmt.Printf("Error downloading: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Bad status: %s\n", resp.Status)
		os.Exit(1)
	}

	tmpFile, err := os.CreateTemp("", "download-*.zip")
	if err != nil {
		fmt.Printf("Error creating temp file: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		fmt.Printf("Error saving download: %v\n", err)
		os.Exit(1)
	}

	reader, err := zip.OpenReader(tmpFile.Name())
	if err != nil {
		fmt.Printf("Error opening zip: %v\n", err)
		os.Exit(1)
	}
	defer reader.Close()

	for _, file := range reader.File {
		path := filepath.Join(*outputFlag, file.Name)

		if file.FileInfo().IsDir() {
			os.MkdirAll(path, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
			fmt.Printf("Error creating directory for file: %v\n", err)
			os.Exit(1)
		}

		dstFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			fmt.Printf("Error creating file: %v\n", err)
			os.Exit(1)
		}

		srcFile, err := file.Open()
		if err != nil {
			dstFile.Close()
			fmt.Printf("Error opening zip entry: %v\n", err)
			os.Exit(1)
		}

		_, err = io.Copy(dstFile, srcFile)
		dstFile.Close()
		srcFile.Close()
		if err != nil {
			fmt.Printf("Error extracting file: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println("UI Extracted successfully!")
}
