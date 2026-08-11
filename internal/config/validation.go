package config

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/go-playground/validator/v10"
)

func validateTaggedFields(cfg Config) []string {
	validate := validator.New(validator.WithRequiredStructEnabled())
	validate.RegisterTagNameFunc(func(field reflect.StructField) string {
		if key := field.Tag.Get("koanf"); key != "" && key != "-" {
			return key
		}
		return field.Name
	})
	if err := validate.Struct(cfg); err != nil {
		var problems []string
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			for _, fieldErr := range validationErrors {
				path := strings.TrimPrefix(fieldErr.Namespace(), "Config.")
				path = strings.ReplaceAll(path, ".", " ")
				problems = append(problems, fmt.Sprintf("%s failed %s validation", path, fieldErr.Tag()))
			}
		} else {
			problems = append(problems, err.Error())
		}
		sort.Strings(problems)
		return problems
	}
	return nil
}
