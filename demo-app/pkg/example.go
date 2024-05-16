package pkg

import (
	"errors"
	"time"
)

// test complex returns
func DoAThing(willError bool) (string, bool, error) {
	time.Sleep(200 * time.Millisecond)
	if willError {
		return "thing not done", false, errors.New("this is an error")
	}

	return "thing complete", true, nil
}
