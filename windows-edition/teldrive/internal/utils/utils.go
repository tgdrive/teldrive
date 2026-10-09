package utils

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func CamelToSnake(input string) string {
	re := regexp.MustCompile("([a-z0-9])([A-Z])")
	snake := re.ReplaceAllString(input, "${1}_${2}")
	return strings.ToLower(snake)
}

func Ptr[T any](t T) *T {
	return &t
}

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func ExecutableDir() string {
	path, _ := os.Executable()
	return filepath.Dir(path)
}

func Filter[T any](slice []T, predicate func(T) bool) []T {
	var result []T
	for _, v := range slice {
		if predicate(v) {
			result = append(result, v)
		}
	}
	return result
}

func Map[T any, U any](slice []T, mapper func(T) U) []U {
	var result []U
	for _, v := range slice {
		result = append(result, mapper(v))
	}
	return result
}

func Find[T any](slice []T, predicate func(T) bool) (T, bool) {
	for _, v := range slice {
		if predicate(v) {
			return v, true
		}
	}
	var zero T
	return zero, false
}
