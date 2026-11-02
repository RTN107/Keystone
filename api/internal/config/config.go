// Package config loads and validates environment configuration. It fails fast
// with every problem listed at once, so a bad deploy says what to fix.
package config

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// TeamSpec is one team to create at startup: its name, and the capital letters that
// start its employee numbers (DEL gives DEL-001, DEL-002).
type TeamSpec struct {
	Name   string
	Prefix string
}

type Config struct {
	Port string

	// DatabaseURL is the connection string the API serves requests with. On Neon
	// this is the pooled string.
	DatabaseURL string
	// DirectDatabaseURL is optional. Migrations take a session-level lock, which
	// does not survive a transaction pooler, so they run on the direct host.
	DirectDatabaseURL string

	JWTSecret string
	WebOrigin string

	// ReadOnly makes the server refuse every write except sign-in and asking.
	ReadOnly bool

	// AnthropicAPIKey is read from KEYSTONE_ANTHROPIC_KEY first, then from
	// ANTHROPIC_API_KEY. Some hosts reserve the second name and do not pass it
	// through, so the project-specific name wins when both are set.
	AnthropicAPIKey string
	// AnthropicModel has no default, because model names are retired over time and a default
	// would go stale. It is required whenever a key is set.
	AnthropicModel string
	VoyageAPIKey   string
	VoyageBaseURL  string
	// VoyageModel is empty to use the built-in default. The model must be able to return
	// 1024-dimensional vectors, because that is the size of the database column.
	VoyageModel string

	// Teams is read from KEYSTONE_TEAMS and created at startup if missing.
	Teams []TeamSpec
	// WarmQuestions are embedded once at startup so they are fast the first time they are
	// asked. Empty by default.
	WarmQuestions []string
	// RelevanceFloor and BlockMargin override the retrieval thresholds. Zero means: use the
	// built-in default.
	RelevanceFloor float64
	BlockMargin    float64

	// Public-link protection for Ask. AsksPerIP is how many questions one client address
	// may ask in ten minutes. DailyAskCap is how many questions the whole deployment answers
	// in 24 hours before it rests. ProxyHops is how many trusted proxies sit in front of the
	// API: the client address is that many entries from the right of X-Forwarded-For, because
	// the entries on the left are written by the client and can be forged.
	AsksPerIP   int
	DailyAskCap int
	ProxyHops   int
}

const minJWTSecretLen = 32

// DefaultVoyageBaseURL is Voyage's embeddings API as served through MongoDB. The
// /embeddings path under it was confirmed to exist (401 without credentials, 404
// for a wrong path).
const DefaultVoyageBaseURL = "https://ai.mongodb.com/v1"

func Load(getenv func(string) string) (Config, error) {
	c := Config{
		Port:              orDefault(getenv("PORT"), "8080"),
		DatabaseURL:       getenv("DATABASE_URL"),
		DirectDatabaseURL: getenv("DIRECT_DATABASE_URL"),
		JWTSecret:         getenv("JWT_SECRET"),
		WebOrigin:         strings.TrimRight(orDefault(getenv("WEB_ORIGIN"), "http://localhost:5173"), "/"),
		ReadOnly:          parseBool(getenv("KEYSTONE_READ_ONLY")),
		AnthropicAPIKey:   firstNonEmpty(getenv("KEYSTONE_ANTHROPIC_KEY"), getenv("ANTHROPIC_API_KEY")),
		AnthropicModel:    strings.TrimSpace(getenv("ANTHROPIC_MODEL")),
		VoyageAPIKey:      getenv("VOYAGE_API_KEY"),
		VoyageBaseURL:     strings.TrimRight(orDefault(getenv("VOYAGE_BASE_URL"), DefaultVoyageBaseURL), "/"),
		VoyageModel:       strings.TrimSpace(getenv("VOYAGE_MODEL")),
		WarmQuestions:     splitList(getenv("KEYSTONE_WARM_QUESTIONS"), "|"),
		AsksPerIP:         positiveInt(getenv("KEYSTONE_ASKS_PER_IP"), 6),
		DailyAskCap:       positiveInt(getenv("KEYSTONE_DAILY_ASK_CAP"), 150),
		ProxyHops:         positiveInt(getenv("KEYSTONE_PROXY_HOPS"), 1),
	}

	var problems []error
	teams, err := ParseTeams(getenv("KEYSTONE_TEAMS"))
	if err != nil {
		problems = append(problems, err)
	}
	c.Teams = teams
	var perr error
	if c.RelevanceFloor, perr = optionalUnit("KEYSTONE_RELEVANCE_FLOOR", getenv("KEYSTONE_RELEVANCE_FLOOR")); perr != nil {
		problems = append(problems, perr)
	}
	if c.BlockMargin, perr = optionalUnit("KEYSTONE_BLOCK_MARGIN", getenv("KEYSTONE_BLOCK_MARGIN")); perr != nil {
		problems = append(problems, perr)
	}
	if c.AnthropicAPIKey != "" && c.AnthropicModel == "" {
		problems = append(problems, errors.New("ANTHROPIC_MODEL is required when an Anthropic key is set: name the Claude model that should write answers"))
	}
	if c.DatabaseURL == "" {
		problems = append(problems, errors.New("DATABASE_URL is required"))
	}
	if len(c.JWTSecret) < minJWTSecretLen {
		problems = append(problems, errors.New("JWT_SECRET is required and must be at least 32 characters (generate one with: openssl rand -hex 32)"))
	}
	if u, err := url.Parse(c.WebOrigin); err != nil || u.Scheme == "" || u.Host == "" || u.Path != "" {
		problems = append(problems, errors.New("WEB_ORIGIN must be an origin such as https://example.vercel.app, with no path"))
	}
	return c, errors.Join(problems...)
}

// MigrationURL is the connection string migrations run on.
func (c Config) MigrationURL() string {
	if c.DirectDatabaseURL != "" {
		return c.DirectDatabaseURL
	}
	return DirectFromPooled(c.DatabaseURL)
}

// EmbeddingsEnabled reports whether ingestion and retrieval can run.
func (c Config) EmbeddingsEnabled() bool { return c.VoyageAPIKey != "" && c.VoyageBaseURL != "" }

// AnswersEnabled reports whether answer generation can run.
func (c Config) AnswersEnabled() bool { return c.AnthropicAPIKey != "" }

// DirectFromPooled turns a Neon pooled host (ep-name-pooler.region...) into the
// direct host by removing the "-pooler" suffix. Anything else is returned as is.
func DirectFromPooled(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	first, rest, ok := strings.Cut(u.Hostname(), ".")
	if !ok || !strings.HasSuffix(first, "-pooler") {
		return raw
	}
	host := strings.TrimSuffix(first, "-pooler") + "." + rest
	if p := u.Port(); p != "" {
		host += ":" + p
	}
	u.Host = host
	return u.String()
}

var teamPrefixRE = regexp.MustCompile(`^[A-Z]{2,4}$`)

// ParseTeams reads "Name:PREFIX;Name:PREFIX". The prefix is whatever follows the last colon,
// so a name may itself contain a colon. Every problem is reported, not just the first.
func ParseTeams(raw string) ([]TeamSpec, error) {
	var (
		out        []TeamSpec
		errs       []error
		seenName   = map[string]bool{}
		seenPrefix = map[string]bool{}
	)
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		i := strings.LastIndex(part, ":")
		if i < 0 {
			errs = append(errs, fmt.Errorf("KEYSTONE_TEAMS: %q needs the form Name:PREFIX", part))
			continue
		}
		name, prefix := strings.TrimSpace(part[:i]), strings.TrimSpace(part[i+1:])
		switch {
		case name == "":
			errs = append(errs, fmt.Errorf("KEYSTONE_TEAMS: %q has no team name", part))
		case !teamPrefixRE.MatchString(prefix):
			errs = append(errs, fmt.Errorf("KEYSTONE_TEAMS: the prefix %q for %q must be 2 to 4 capital letters", prefix, name))
		case seenName[strings.ToLower(name)]:
			errs = append(errs, fmt.Errorf("KEYSTONE_TEAMS: %q is listed twice", name))
		case seenPrefix[prefix]:
			errs = append(errs, fmt.Errorf("KEYSTONE_TEAMS: the prefix %q is used by more than one team", prefix))
		default:
			seenName[strings.ToLower(name)] = true
			seenPrefix[prefix] = true
			out = append(out, TeamSpec{Name: name, Prefix: prefix})
		}
	}
	return out, errors.Join(errs...)
}

// splitList splits on sep, trims each item and drops empty ones.
func splitList(raw, sep string) []string {
	var out []string
	for _, item := range strings.Split(raw, sep) {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// optionalUnit reads a number strictly between 0 and 1. Unset gives 0, meaning "use the
// default". A bad value is an error and not a silent fallback, because these two numbers
// decide what is refused and what is answered.
func optionalUnit(name, raw string) (float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(f) || f <= 0 || f >= 1 {
		return 0, fmt.Errorf("%s must be a number above 0 and below 1, for example 0.40", name)
	}
	return f, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// positiveInt reads a whole number above zero, and falls back to def for anything else
// (unset, not a number, zero or negative), so a typo can never switch a limit off.
func positiveInt(raw string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return def
	}
	return n
}
