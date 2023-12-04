package http_range

import (
	"errors"
	"strconv"
	"strings"
)

type Range struct {
	Start int64
	End   int64
}

var (
	ErrNoOverlap = errors.New("invalid range: failed to overlap")

	ErrInvalid = errors.New("invalid range")
)

func Parse(header string, size int64) ([]*Range, error) {
	index := strings.Index(header, "=")

	if index == -1 {
		return nil, ErrInvalid
	}

	size64 := int64(size)
	arr := strings.Split(header[index+1:], ",")
	ranges := make([]*Range, 0, len(arr))

	for _, value := range arr {
		r := strings.Split(value, "-")
		start, startErr := strconv.ParseInt(r[0], 10, 64)
		end, endErr := strconv.ParseInt(r[1], 10, 64)

		if startErr != nil && endErr != nil {
			continue
		}

		// -nnn and nnn-
		if startErr != nil {
			start = size64 - end
			end = size64 - 1
		} else if endErr != nil {
			end = size64 - 1
		}

		if end >= size64 {
			end = size64 - 1
		}

		if start > end || start < 0 {
			continue
		}

		ranges = append(ranges, &Range{
			Start: start,
			End:   end,
		})
	}

	if len(ranges) == 0 {
		return nil, ErrNoOverlap
	}

	return ranges, nil
}
