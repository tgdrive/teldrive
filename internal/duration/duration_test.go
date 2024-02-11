package duration

import "testing"

func TestDate(t *testing.T) {
	res, _ := ParseDuration("15h2m10s")
	_ = res
}
