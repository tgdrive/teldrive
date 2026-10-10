// cli-schema exports the same metadata used by the server's CLI flags.
package main

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/tgdrive/teldrive/internal/config"
)

type field struct {
	Path        string `json:"path"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Description string `json:"description"`
}

var first = regexp.MustCompile("(.)([A-Z][a-z]+)")
var all = regexp.MustCompile("([a-z0-9])([A-Z])")

func walk(t reflect.Type, prefix string, fields *[]field) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		key := f.Tag.Get("koanf")
		if key == "" {
			key = strings.ToLower(all.ReplaceAllString(first.ReplaceAllString(f.Name, "${1}-${2}"), "${1}-${2}"))
		}
		if prefix != "" {
			key = prefix + "." + key
		}
		if f.Type.Kind() == reflect.Struct {
			walk(f.Type, key, fields)
			continue
		}
		kind := f.Type.Kind().String()
		if f.Type == reflect.TypeOf(time.Duration(0)) {
			kind = "duration"
		}
		*fields = append(*fields, field{key, kind, f.Tag.Get("default"), f.Tag.Get("description")})
	}
}

func main() {
	fields := []field{}
	walk(reflect.TypeFor[config.ServerCmdConfig](), "", &fields)
	if err := json.NewEncoder(os.Stdout).Encode(fields); err != nil {
		panic(err)
	}
}
