package duration

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
)

type Duration time.Duration

const DurationOff = Duration((1 << 63) - 1)

func (d *Duration) String() string {
	if *d == DurationOff {
		return "off"
	}

	ageSuffix := &ageSuffixes[0]
	if math.Abs(float64(*d)) >= float64(ageSuffix.Multiplier) {
		timeUnits := float64(*d) / float64(ageSuffix.Multiplier)
		return strconv.FormatFloat(timeUnits, 'f', -1, 64) + ageSuffix.Suffix
	}
	return time.Duration(*d).String()
}

func (d *Duration) Set(s string) error {
	v, err := ParseDuration(s)
	*d = Duration(v)
	return err
}

var ageSuffixes = []struct {
	Suffix     string
	Multiplier time.Duration
}{
	{Suffix: "d", Multiplier: time.Hour * 24},
	{Suffix: "w", Multiplier: time.Hour * 24 * 7},
	{Suffix: "M", Multiplier: time.Hour * 24 * 30},
	{Suffix: "y", Multiplier: time.Hour * 24 * 365},
	{Suffix: "", Multiplier: time.Second},
}

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

func parseDurationFromNow(age string) (d time.Duration, err error) {
	if age == "off" {
		return time.Duration(DurationOff), nil
	}

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

func newDurationValue(val time.Duration, p *time.Duration) *Duration {
	*p = val
	return (*Duration)(p)
}

func DurationVar(f *pflag.FlagSet, p *time.Duration, name string, value time.Duration, usage string) {
	f.VarP(newDurationValue(value, p), name, "", usage)
}

func ParseDuration(age string) (time.Duration, error) {
	return parseDurationFromNow(age)
}

func (d *Duration) UnmarshalText(text []byte) error {
	if err := d.Set(string(text)); err != nil {
		return err
	}
	return nil
}

func (d *Duration) Type() string {
	return "Duration"
}
