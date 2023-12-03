package utils

import (
	"os"
	"regexp"
	"strings"
	"time"

	"reflect"

	"unicode"
)

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
func Int64Pointer(b int64) *int64 {
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
