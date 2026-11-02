// Package auth holds credentials, tokens, the principal carried through a
// request, and the middleware that establishes it.
package auth

import (
	"errors"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

const (
	bcryptCost     = 12
	MinPasswordLen = 10
	// bcrypt ignores everything past 72 bytes, so longer passwords are refused
	// rather than silently truncated.
	maxPasswordBytes = 72
)

var (
	ErrPasswordTooShort = errors.New("Passwords need at least 10 characters.")
	ErrPasswordTooLong  = errors.New("Passwords can be at most 72 bytes long.")
)

func ValidatePassword(p string) error {
	switch {
	case len([]rune(p)) < MinPasswordLen:
		return ErrPasswordTooShort
	case len(p) > maxPasswordBytes:
		return ErrPasswordTooLong
	}
	return nil
}

func HashPassword(p string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(p), bcryptCost)
	return string(h), err
}

func CheckPassword(hash, p string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil
}

var (
	dummyOnce sync.Once
	dummyHash string
)

// BurnPasswordCheck spends the same time as a real check. Login calls it for an
// unknown employee number so response time does not reveal which numbers exist.
func BurnPasswordCheck(p string) {
	dummyOnce.Do(func() { dummyHash, _ = HashPassword("keystone-timing-equaliser") })
	_ = CheckPassword(dummyHash, p)
}
