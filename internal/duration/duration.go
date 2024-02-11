package duration

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
)

// Duration is a time.Duration with some more parsing options
type Duration time.Duration

// DurationOff is the default value for flags which can be turned off
const DurationOff = Duration((1 << 63) - 1)

// Turn Duration into a string
func (d Duration) String() string {
	if d == DurationOff {
		return "off"
	}
	for i := len(ageSuffixes) - 2; i >= 0; i-- {
		ageSuffix := &ageSuffixes[i]
		if math.Abs(float64(d)) >= float64(ageSuffix.Multiplier) {
			timeUnits := float64(d) / float64(ageSuffix.Multiplier)
			return strconv.FormatFloat(timeUnits, 'f', -1, 64) + ageSuffix.Suffix
		}
	}
	return time.Duration(d).String()
}

// IsSet returns if the duration is != DurationOff
func (d Duration) IsSet() bool {
	return d != DurationOff
}

// We use time conventions
var ageSuffixes = []struct {
	Suffix     string
	Multiplier time.Duration
}{
	{Suffix: "d", Multiplier: time.Hour * 24},
	{Suffix: "w", Multiplier: time.Hour * 24 * 7},
	{Suffix: "M", Multiplier: time.Hour * 24 * 30},
	{Suffix: "y", Multiplier: time.Hour * 24 * 365},

	// Default to second
	{Suffix: "", Multiplier: time.Second},
}

// parse the age as suffixed ages
func parseDurationSuffixes(age string) (time.Duration, error) {
	var period float64

	for _, ageSuffix := range ageSuffixes {
		if strings.HasSuffix(age, ageSuffix.Suffix) {
			numberString := age[:len(age)-len(ageSuffix.Suffix)]
			var err error
			period, err = strconv.ParseFloat(numberString, 64)
			if err != nil {
				return time.Duration(0), err
			}
			period *= float64(ageSuffix.Multiplier)
			break
		}
	}

	return time.Duration(period), nil
}

// parseDurationFromNow parses a duration string. Allows ParseDuration to match the time
// package and easier testing within the fs package.
func parseDurationFromNow(age string) (d time.Duration, err error) {
	if age == "off" {
		return time.Duration(DurationOff), nil
	}

	// Attempt to parse as a time.Duration first
	d, err = time.ParseDuration(age)
	if err == nil {
		return d, nil
	}

	d, err = parseDurationSuffixes(age)
	if err == nil {
		return d, nil
	}

	return d, err
}

func ParseDuration(age string) (time.Duration, error) {
	return parseDurationFromNow(age)
}

func (d Duration) Type() string {
	return "Duration"
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		*d = Duration(value)
		return nil
	case string:
		var err error
		dur, err := ParseDuration(value)
		*d = Duration(dur)
		if err != nil {
			return err
		}
		return nil
	default:
		return errors.New("invalid duration")
	}
}
