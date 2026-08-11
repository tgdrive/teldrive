package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/maps"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/pflag"

	"github.com/tgdrive/teldrive/v2/internal/size"
)

const defaultConfigPath = "$HOME/.teldrive/config.toml"

var (
	matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
	matchAllCap   = regexp.MustCompile("([a-z0-9])([A-Z])")
)

type Loader struct {
	lookup  func(string) (string, bool)
	homeDir func() (string, error)
	flagMap map[string]string
	envMap  map[string]string
}

func NewLoader() *Loader {
	return newLoader(os.LookupEnv, os.UserHomeDir)
}

func newLoader(lookup func(string) (string, bool), homeDir func() (string, error)) *Loader {
	return &Loader{
		lookup:  lookup,
		homeDir: homeDir,
		flagMap: make(map[string]string),
		envMap:  make(map[string]string),
	}
}

// RegisterFlags exposes every configuration leaf as a kebab-case pflag. The
// same path maps to a TOML/YAML key and TELDRIVE_ environment variable.
func (l *Loader) RegisterFlags(flags *pflag.FlagSet) {
	if flags == nil {
		return
	}
	flags.StringP("config", "c", "", "Config file path (default "+defaultConfigPath+")")
	defaults := Default()
	l.registerStruct(flags, "", reflect.ValueOf(defaults), reflect.TypeOf(defaults))
}

// Load applies sources in the same precedence order as the original TelDrive:
// defaults < config file < environment variables < explicitly changed flags.
func (l *Loader) Load(flags *pflag.FlagSet) (Config, error) {
	if l == nil || l.lookup == nil || l.homeDir == nil {
		return Config{}, fmt.Errorf("%w: configuration loader is not initialized", ErrInvalid)
	}
	if flags == nil {
		return Config{}, fmt.Errorf("%w: flag set is required", ErrInvalid)
	}

	k := koanf.New(".")
	if err := k.Load(staticProvider{values: defaultsMap(Default())}, nil); err != nil {
		return Config{}, fmt.Errorf("load configuration defaults: %w", err)
	}

	configPath, err := l.resolveConfigPath(flags)
	if err != nil {
		return Config{}, err
	}
	if configPath != "" {
		parser, err := parserForPath(configPath)
		if err != nil {
			return Config{}, err
		}
		if err := k.Load(file.Provider(configPath), parser); err != nil {
			return Config{}, fmt.Errorf("read config file %q: %w", configPath, err)
		}
	}

	l.envMap = make(map[string]string)
	l.generateEnvMap(reflect.TypeOf(Config{}), "", "")
	if err := k.Load(staticProvider{values: l.environmentValues()}, nil); err != nil {
		return Config{}, fmt.Errorf("load environment configuration: %w", err)
	}
	if err := k.Load(&flagProvider{flags: flags, flagMap: l.flagMap, onlyChanged: true}, nil); err != nil {
		return Config{}, fmt.Errorf("load command-line configuration: %w", err)
	}

	var cfg Config
	unmarshal := koanf.UnmarshalConf{
		Tag: "koanf",
		DecoderConfig: &mapstructure.DecoderConfig{
			MatchName: func(mapKey, fieldName string) bool {
				return normalizeKey(mapKey) == normalizeKey(fieldName)
			},
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				mapstructure.StringToTimeDurationHookFunc(),
				mapstructure.StringToSliceHookFunc(","),
				decodeSize,
				decodeEncryptionKeys,
			),
			Result:           &cfg,
			WeaklyTypedInput: true,
		},
	}
	if err := k.UnmarshalWithConf("", &cfg, unmarshal); err != nil {
		return Config{}, fmt.Errorf("%w: decode configuration: %v", ErrInvalid, err)
	}
	if cfg.Encryption.Keys == nil {
		cfg.Encryption.Keys = map[int32]string{}
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Load is retained for library callers that only need defaults, auto-discovered
// config files, and environment variables. The executable uses Loader directly
// so explicit pflags retain highest precedence.
func Load() (Config, error) {
	loader := NewLoader()
	flags := pflag.NewFlagSet("teldrive", pflag.ContinueOnError)
	loader.RegisterFlags(flags)
	return loader.Load(flags)
}

func LoadFrom(lookup func(string) (string, bool)) (Config, error) {
	if lookup == nil {
		return Config{}, fmt.Errorf("%w: environment lookup is nil", ErrInvalid)
	}
	loader := newLoader(lookup, func() (string, error) { return "", nil })
	flags := pflag.NewFlagSet("teldrive", pflag.ContinueOnError)
	loader.RegisterFlags(flags)
	return loader.Load(flags)
}

func (l *Loader) resolveConfigPath(flags *pflag.FlagSet) (string, error) {
	if flag := flags.Lookup("config"); flag != nil && flag.Value.String() != "" {
		path := flag.Value.String()
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("stat config file %q: %w", path, err)
		}
		return path, nil
	}
	home, err := l.homeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	candidates := []string{}
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".teldrive", "config.toml"),
			filepath.Join(home, ".teldrive", "config.yaml"),
			filepath.Join(home, ".teldrive", "config.yml"),
		)
	}
	candidates = append(candidates, "config.toml", "config.yaml", "config.yml")
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", nil
}

func parserForPath(path string) (koanf.Parser, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".toml":
		return toml.Parser(), nil
	case ".yaml", ".yml":
		return yaml.Parser(), nil
	default:
		return nil, fmt.Errorf("%w: config file must use .toml, .yaml, or .yml", ErrInvalid)
	}
}

func (l *Loader) environmentValues() map[string]any {
	flat := make(map[string]any)
	keys := make([]string, 0, len(l.envMap))
	for key := range l.envMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, envKey := range keys {
		if value, ok := l.lookup(envPrefix + envKey); ok {
			flat[l.envMap[envKey]] = value
		}
	}
	return maps.Unflatten(flat, ".")
}

func (l *Loader) generateEnvMap(t reflect.Type, path, envPath string) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		key := fieldKey(field)
		childPath := joinPath(path, key)
		childEnv := strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
		if envPath != "" {
			childEnv = envPath + "_" + childEnv
		}
		if isNestedStruct(field.Type) {
			l.generateEnvMap(field.Type, childPath, childEnv)
			continue
		}
		l.envMap[childEnv] = childPath
	}
}

func (l *Loader) registerStruct(flags *pflag.FlagSet, path string, value reflect.Value, t reflect.Type) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldValue := value.Field(i)
		key := joinPath(path, fieldKey(field))
		if isNestedStruct(field.Type) {
			l.registerStruct(flags, key, fieldValue, field.Type)
			continue
		}
		name := strings.ReplaceAll(key, ".", "-")
		l.flagMap[name] = key
		description := field.Tag.Get("description")
		if description == "" {
			description = "Set " + key
		}
		switch {
		case field.Type == reflect.TypeOf(time.Duration(0)):
			flags.Duration(name, time.Duration(fieldValue.Int()), description)
		case field.Type == reflect.TypeOf(size.Size(0)):
			flags.String(name, fieldValue.Interface().(size.Size).String(), description)
		case field.Type == reflect.TypeOf(map[int32]string{}):
			flags.String(name, formatEncryptionKeys(fieldValue.Interface().(map[int32]string)), description)
		case field.Type == reflect.TypeOf([]string{}):
			flags.StringSlice(name, fieldValue.Interface().([]string), description)
		case field.Type.Kind() == reflect.String:
			flags.String(name, fieldValue.String(), description)
		case field.Type.Kind() == reflect.Bool:
			flags.Bool(name, fieldValue.Bool(), description)
		case field.Type.Kind() == reflect.Int:
			flags.Int(name, int(fieldValue.Int()), description)
		case field.Type.Kind() == reflect.Int32:
			flags.Int32(name, int32(fieldValue.Int()), description)
		case field.Type.Kind() == reflect.Int64:
			flags.Int64(name, fieldValue.Int(), description)
		}
	}
}

func defaultsMap(cfg Config) map[string]any {
	return structMap(reflect.ValueOf(cfg), reflect.TypeOf(cfg))
}

func structMap(value reflect.Value, t reflect.Type) map[string]any {
	result := make(map[string]any)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldValue := value.Field(i)
		key := fieldKey(field)
		switch {
		case isNestedStruct(field.Type):
			result[key] = structMap(fieldValue, field.Type)
		case field.Type == reflect.TypeOf(time.Duration(0)):
			result[key] = time.Duration(fieldValue.Int()).String()
		case field.Type == reflect.TypeOf(size.Size(0)):
			result[key] = fieldValue.Interface().(size.Size).String()
		case field.Type == reflect.TypeOf(map[int32]string{}):
			result[key] = formatEncryptionKeys(fieldValue.Interface().(map[int32]string))
		default:
			result[key] = fieldValue.Interface()
		}
	}
	return result
}

func decodeSize(_ reflect.Type, to reflect.Type, data any) (any, error) {
	if to != reflect.TypeOf(size.Size(0)) {
		return data, nil
	}
	switch value := data.(type) {
	case string:
		return size.Parse(value)
	case int:
		return size.Size(value), nil
	case int64:
		return size.Size(value), nil
	case float64:
		return size.Size(value), nil
	default:
		return data, nil
	}
}

func decodeEncryptionKeys(from reflect.Type, to reflect.Type, data any) (any, error) {
	if to != reflect.TypeOf(map[int32]string{}) {
		return data, nil
	}
	switch value := data.(type) {
	case string:
		return parseEncryptionKeys(value)
	case map[string]any:
		keys := make(map[int32]string, len(value))
		for rawVersion, rawKey := range value {
			version, err := strconv.ParseInt(rawVersion, 10, 32)
			key, ok := rawKey.(string)
			if err != nil || !ok || version <= 0 || strings.TrimSpace(key) == "" {
				return nil, fmt.Errorf("%w: an encryption key entry is invalid", ErrInvalid)
			}
			keys[int32(version)] = key
		}
		return keys, nil
	default:
		_ = from
		return data, nil
	}
}

func formatEncryptionKeys(keys map[int32]string) string {
	versions := make([]int, 0, len(keys))
	for version := range keys {
		versions = append(versions, int(version))
	}
	sort.Ints(versions)
	parts := make([]string, 0, len(versions))
	for _, version := range versions {
		parts = append(parts, fmt.Sprintf("%d:%s", version, keys[int32(version)]))
	}
	return strings.Join(parts, ",")
}

func fieldKey(field reflect.StructField) string {
	if tag := field.Tag.Get("koanf"); tag != "" {
		return tag
	}
	return toKebab(field.Name)
}

func toKebab(value string) string {
	kebab := matchFirstCap.ReplaceAllString(value, "${1}-${2}")
	kebab = matchAllCap.ReplaceAllString(kebab, "${1}-${2}")
	return strings.ToLower(kebab)
}

func normalizeKey(value string) string {
	value = strings.ReplaceAll(value, "-", "")
	value = strings.ReplaceAll(value, "_", "")
	return strings.ToLower(value)
}

func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func isNestedStruct(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && t != reflect.TypeOf(time.Duration(0)) && t != reflect.TypeOf(size.Size(0))
}

type staticProvider struct{ values map[string]any }

func (p staticProvider) Read() (map[string]any, error) { return p.values, nil }
func (p staticProvider) ReadBytes() ([]byte, error)    { return nil, nil }

type flagProvider struct {
	flags       *pflag.FlagSet
	flagMap     map[string]string
	onlyChanged bool
}

func (p *flagProvider) Read() (map[string]any, error) {
	flat := make(map[string]any)
	p.flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Name == "config" || (p.onlyChanged && !flag.Changed) {
			return
		}
		key, ok := p.flagMap[flag.Name]
		if !ok {
			return
		}
		flat[key] = flag.Value.String()
	})
	return maps.Unflatten(flat, "."), nil
}

func (p *flagProvider) ReadBytes() ([]byte, error) { return nil, nil }
