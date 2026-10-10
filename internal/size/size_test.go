package size

import "testing"

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value string
		want  Size
	}{
		{value: "0", want: 0},
		{value: "1KB", want: 1 << 10},
		{value: "1.5MiB", want: 3 << 19},
		{value: "2 gb", want: 2 << 30},
		{value: "1TB", want: 1 << 40},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(test.value)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", test.value, err)
			}
			if got != test.want {
				t.Fatalf("Parse(%q) = %d, want %d", test.value, got, test.want)
			}
		})
	}
}

func TestParseRejectsInvalidValues(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"", "-1MB", "many", "1XB", "999999999999999999999TB"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse(value); err == nil {
				t.Fatalf("Parse(%q) unexpectedly succeeded", value)
			}
		})
	}
}

func TestString(t *testing.T) {
	t.Parallel()
	for value, want := range map[Size]string{
		0:        "0B",
		1 << 10:  "1KB",
		32 << 20: "32MB",
		3 << 30:  "3GB",
		1536:     "1536B",
	} {
		if got := value.String(); got != want {
			t.Fatalf("Size(%d).String() = %q, want %q", value, got, want)
		}
	}
}
