package auth

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const TokenTTL = 12 * time.Hour

var ErrInvalidToken = errors.New("invalid token")

// Issuer signs and verifies session tokens. A token carries only the employee id
// and an expiry. Role and team are always read from the database per request, so
// a change to either takes effect immediately and a token never holds stale access.
type Issuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewIssuer(secret string) *Issuer {
	return &Issuer{secret: []byte(secret), ttl: TokenTTL, now: time.Now}
}

func (i *Issuer) Issue(employeeID int64) (string, time.Time, error) {
	exp := i.now().Add(i.ttl)
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(employeeID, 10),
		IssuedAt:  jwt.NewNumericDate(i.now()),
		ExpiresAt: jwt.NewNumericDate(exp),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
	return signed, exp, err
}

func (i *Issuer) Parse(token string) (int64, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(token, &claims,
		func(*jwt.Token) (any, error) { return i.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(i.now),
	)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return 0, ErrInvalidToken
	}
	return id, nil
}
