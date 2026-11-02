package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

const (
	RoleAdmin    = "admin"
	RoleEmployee = "employee"
)

// Principal is who is making a request, as the database says right now.
type Principal struct {
	ID       int64
	Number   string
	Name     string
	Role     string
	TeamID   *int64 // nil for admins
	TeamName *string
}

func (p Principal) IsAdmin() bool { return p.Role == RoleAdmin }

type ctxKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

// ErrUnknownEmployee is returned by a Lookup when the token names someone who no
// longer exists.
var ErrUnknownEmployee = errors.New("unknown employee")

type Lookup func(ctx context.Context, id int64) (Principal, error)

// Fail is how middleware reports a refusal. The HTTP layer supplies it so every
// error in the API shares one JSON shape.
type Fail func(w http.ResponseWriter, status int, code, message string)

// Authenticate resolves the bearer token to a Principal and loads it fresh from
// the database. Without a valid token the request stops here.
func Authenticate(issuer *Issuer, lookup Lookup, fail Fail) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || token == "" {
				fail(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
				return
			}
			id, err := issuer.Parse(token)
			if err != nil {
				fail(w, http.StatusUnauthorized, "unauthenticated", "Your session has ended. Sign in again to continue.")
				return
			}
			p, err := lookup(r.Context(), id)
			if err != nil {
				if errors.Is(err, ErrUnknownEmployee) {
					fail(w, http.StatusUnauthorized, "unauthenticated", "Your session has ended. Sign in again to continue.")
				} else {
					fail(w, http.StatusInternalServerError, "internal", "Something went wrong on our side. Try again in a moment.")
				}
				return
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

// RequireRole must run after Authenticate.
func RequireRole(role string, fail Fail) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFrom(r.Context())
			if !ok || p.Role != role {
				fail(w, http.StatusForbidden, "forbidden", "Your account does not have access to this.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
