package config

import (
	"reflect"
	"testing"
	"time"

	"github.com/spf13/pflag"

	"github.com/tgdrive/teldrive/v2/internal/size"
)

func TestDecodeSizeVariants(t *testing.T) {
	t.Parallel()
	target := reflect.TypeOf(size.Size(0))
	other := reflect.TypeOf("")

	if got, err := decodeSize(nil, other, "1MiB"); err != nil || got != "1MiB" {
		t.Fatalf("non-size decode = %#v, %v", got, err)
	}
	tests := []struct {
		input any
		want  size.Size
	}{
		{input: "1MiB", want: size.Size(1 << 20)},
		{input: int(12), want: 12},
		{input: int64(13), want: 13},
		{input: float64(14), want: 14},
	}
	for _, test := range tests {
		got, err := decodeSize(nil, target, test.input)
		if err != nil || got != test.want {
			t.Fatalf("decodeSize(%#v) = %#v, %v, want %v", test.input, got, err, test.want)
		}
	}
	marker := struct{}{}
	if got, err := decodeSize(nil, target, marker); err != nil || got != marker {
		t.Fatalf("default decode = %#v, %v", got, err)
	}
	if _, err := decodeSize(nil, target, "not-a-size"); err == nil {
		t.Fatal("invalid size was accepted")
	}
}

func TestDecodeEncryptionKeysVariants(t *testing.T) {
	t.Parallel()
	target := reflect.TypeOf(map[int32]string{})
	other := reflect.TypeOf("")
	if got, err := decodeEncryptionKeys(nil, other, "1:key"); err != nil || got != "1:key" {
		t.Fatalf("non-key decode = %#v, %v", got, err)
	}
	got, err := decodeEncryptionKeys(nil, target, "2:beta,1:alpha")
	if err != nil {
		t.Fatal(err)
	}
	keys := got.(map[int32]string)
	if keys[1] != "alpha" || keys[2] != "beta" {
		t.Fatalf("string keys = %#v", keys)
	}
	got, err = decodeEncryptionKeys(nil, target, map[string]any{"3": "gamma"})
	if err != nil || got.(map[int32]string)[3] != "gamma" {
		t.Fatalf("map keys = %#v, %v", got, err)
	}
	for _, invalid := range []map[string]any{
		{"bad": "key"}, {"0": "key"}, {"1": ""}, {"1": 123},
	} {
		if _, err := decodeEncryptionKeys(nil, target, invalid); err == nil {
			t.Fatalf("invalid key map accepted: %#v", invalid)
		}
	}
	marker := 17
	if got, err := decodeEncryptionKeys(nil, target, marker); err != nil || got != marker {
		t.Fatalf("default key decode = %#v, %v", got, err)
	}
	if formatted := formatEncryptionKeys(map[int32]string{2: "beta", 1: "alpha"}); formatted != "1:alpha,2:beta" {
		t.Fatalf("formatEncryptionKeys() = %q", formatted)
	}
}

func TestLoaderProviderAndKeyHelpers(t *testing.T) {
	t.Parallel()
	provider := staticProvider{values: map[string]any{"x": 1}}
	if values, err := provider.Read(); err != nil || values["x"] != 1 {
		t.Fatalf("static Read() = %#v, %v", values, err)
	}
	if data, err := provider.ReadBytes(); err != nil || data != nil {
		t.Fatalf("static ReadBytes() = %#v, %v", data, err)
	}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("name", "value", "")
	flagSource := &flagProvider{flags: flags, flagMap: map[string]string{"name": "section.name"}}
	values, err := flagSource.Read()
	if err != nil || values["section"].(map[string]any)["name"] != "value" {
		t.Fatalf("flag Read() = %#v, %v", values, err)
	}
	if data, err := flagSource.ReadBytes(); err != nil || data != nil {
		t.Fatalf("flag ReadBytes() = %#v, %v", data, err)
	}

	if got := toKebab("HTTPServerURL"); got != "http-server-url" {
		t.Fatalf("toKebab() = %q", got)
	}
	if got := normalizeKey("Some-Key_Name"); got != "somekeyname" {
		t.Fatalf("normalizeKey() = %q", got)
	}
	if got := joinPath("", "key"); got != "key" {
		t.Fatalf("joinPath root = %q", got)
	}
	if got := joinPath("parent", "key"); got != "parent.key" {
		t.Fatalf("joinPath nested = %q", got)
	}
	if !isNestedStruct(reflect.TypeOf(struct{ Value string }{})) || isNestedStruct(reflect.TypeOf(time.Second)) || isNestedStruct(reflect.TypeOf(size.Size(0))) {
		t.Fatal("isNestedStruct() classification is incorrect")
	}
}
