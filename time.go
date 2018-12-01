package gobinder

import (
	"fmt"
	"time"

	"github.com/araddon/dateparse"
)

func ParseISODateTime(val string) (time.Time, error) {
	res, err := time.Parse(
		time.RFC3339,
		val)
	if err == nil {
		return res, nil
	}

	fmt.Printf("parse datetime failed: %+v\n", err)
	thetime, err := dateparse.ParseAny(val)
	return thetime, err
}
