package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/tgdrive/teldrive/v2/internal/config"
	"github.com/tgdrive/teldrive/v2/internal/size"
)

type row struct {
	section     string
	configKey   string
	flag        string
	env         string
	defaultVal  string
	description string
	validation  string
}

func main() {
	cfg := config.Default()
	rows := collect(reflect.ValueOf(cfg), reflect.TypeOf(cfg), "", "")
	sort.Slice(rows, func(i, j int) bool { return rows[i].configKey < rows[j].configKey })

	var b strings.Builder
	b.WriteString("---\ntitle: \"CLI, environment & config reference\"\ndescription: Complete generated mapping of Teldrive config keys to command-line flags and TELDRIVE_ environment variables.\n---\n\n")
	b.WriteString("Generated from the server configuration structs. Precedence: **defaults < config file < environment < explicit CLI flags**.\n\n")
	b.WriteString("Name mapping example: `http.address` → `TELDRIVE_HTTP_ADDRESS` → `--http-address`. Slices use comma-separated values; encryption maps use `version:key` entries.\n\n")
	b.WriteString("The **Default** column is the runtime default, not a production recommendation.\n\n")

	current := ""
	for _, r := range rows {
		if r.section != current {
			current = r.section
			b.WriteString("## " + title(current) + "\n\n")
			b.WriteString("| Config key | CLI flag | Environment variable | Default | Validation | Description |\n")
			b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | %s | %s | %s |\n",
			escape(r.configKey), escape(r.flag), escape(r.env), codeOrDash(r.defaultVal), codeOrDash(r.validation), escape(r.description))
	}

	out := filepath.Join("docs", "content", "docs", "configuration", "reference.mdx")
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("generated %s (%d settings)\n", out, len(rows))
}

func collect(v reflect.Value, t reflect.Type, path, section string) []row {
	var rows []row
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		key := fieldKey(f)
		if key == "-" {
			continue
		}
		childPath := key
		if path != "" {
			childPath = path + "." + key
		}
		childSection := section
		if childSection == "" {
			childSection = key
		}
		fv := v.Field(i)
		if isNestedStruct(f.Type) {
			rows = append(rows, collect(fv, f.Type, childPath, childSection)...)
			continue
		}
		rows = append(rows, row{
			section:     childSection,
			configKey:   childPath,
			flag:        "--" + strings.ReplaceAll(childPath, ".", "-"),
			env:         "TELDRIVE_" + strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(childPath)),
			defaultVal:  formatValue(fv),
			description: f.Tag.Get("description"),
			validation:  f.Tag.Get("validate"),
		})
	}
	return rows
}

func fieldKey(f reflect.StructField) string {
	if key := f.Tag.Get("koanf"); key != "" {
		return key
	}
	return toKebab(f.Name)
}

func toKebab(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('-')
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

func isNestedStruct(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && t != reflect.TypeOf(time.Duration(0)) && t != reflect.TypeOf(size.Size(0))
}

func formatValue(v reflect.Value) string {
	if v.Type() == reflect.TypeOf(time.Duration(0)) {
		return time.Duration(v.Int()).String()
	}
	if v.Type() == reflect.TypeOf(size.Size(0)) {
		return v.Interface().(size.Size).String()
	}
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return fmt.Sprint(v.Bool())
	case reflect.Int, reflect.Int32, reflect.Int64:
		return fmt.Sprint(v.Int())
	case reflect.Slice:
		if v.Len() == 0 {
			return ""
		}
		parts := make([]string, v.Len())
		for i := range v.Len() {
			parts[i] = fmt.Sprint(v.Index(i).Interface())
		}
		return strings.Join(parts, ",")
	case reflect.Map:
		if v.Len() == 0 {
			return ""
		}
		return fmt.Sprint(v.Interface())
	default:
		return fmt.Sprint(v.Interface())
	}
}

func title(s string) string {
	parts := strings.Split(s, "-")
	for i := range parts {
		if parts[i] == "http" {
			parts[i] = "HTTP"
			continue
		}
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, " ")
}

func codeOrDash(s string) string {
	if s == "" {
		return "—"
	}
	return "`" + escape(s) + "`"
}

func escape(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
