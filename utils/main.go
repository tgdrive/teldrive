package utils

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"reflect"

	"golang.org/x/exp/constraints"

	"unicode"
)

func Max[T constraints.Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func Min[T constraints.Ordered](a, b T) T {
	if a > b {
		return b
	}
	return a
}

func CamelToPascalCase(input string) string {
	var result strings.Builder
	upperNext := true

	for _, char := range input {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			if upperNext {
				result.WriteRune(unicode.ToUpper(char))
				upperNext = false
			} else {
				result.WriteRune(char)
			}
		} else {
			upperNext = true
		}
	}

	return result.String()
}

func CamelToSnake(input string) string {
	re := regexp.MustCompile("([a-z0-9])([A-Z])")
	snake := re.ReplaceAllString(input, "${1}_${2}")
	return strings.ToLower(snake)
}

func GetField(v interface{}, field string) string {
	r := reflect.ValueOf(v)
	f := reflect.Indirect(r).FieldByName(field)
	fieldValue := f.Interface()

	switch v := fieldValue.(type) {
	case string:
		return v
	case time.Time:
		return v.Format(time.RFC3339)
	default:
		return ""
	}
}

func BoolPointer(b bool) *bool {
	return &b
}

func IntPointer(b int) *int {
	return &b
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

func getExecutableDir() string {

	path, _ := os.Executable()

	executableDir := filepath.Dir(path)

	return executableDir
}
