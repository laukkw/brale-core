package store

import (
	"errors"

	"gorm.io/gorm"
)

func isRecordNotFound(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, gorm.ErrRecordNotFound)
}
