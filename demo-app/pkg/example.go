package pkg

import (
	"errors"
	"time"
)

func DoAThing(willError bool) error {
	time.Sleep(200 * time.Millisecond)
	if willError {
		return errors.New("this is an error")
	}

	return nil
}
