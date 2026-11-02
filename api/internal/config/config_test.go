package config

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLoadListsEveryProblem(t *testing.T) {
	_, err := Load(env(map[string]string{"JWT_SECRET": "short", "WEB_ORIGIN": "https://x.app/path"}))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"DATABASE_URL", "JWT_SECRET", "WEB_ORIGIN"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %s: %v", want, err)
		}
	}
}

func TestLoadDefaultsAndFlags(t *testing.T) {
	c, err := Load(env(map[string]string{
		"DATABASE_URL":       "postgres://u:p@h/db",
		"JWT_SECRET":         strings.Repeat("a", 32),
		"KEYSTONE_READ_ONLY": "true",
		"WEB_ORIGIN":         "https://keystone.example.app/",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != "8080" || !c.ReadOnly || c.WebOrigin != "https://keystone.example.app" {
		t.Errorf("unexpected config: %+v", c)
	}
	if c.EmbeddingsEnabled() || c.AnswersEnabled() {
		t.Error("capabilities must be off when keys are absent")
	}
	if c.VoyageBaseURL != DefaultVoyageBaseURL {
		t.Errorf("voyage base url default: %q", c.VoyageBaseURL)
	}
}

func TestDirectFromPooled(t *testing.T) {
	cases := map[string]string{
		"postgres://u:p@ep-quiet-sky-123456-pooler.us-east-2.aws.neon.tech/db?sslmode=require": "postgres://u:p@ep-quiet-sky-123456.us-east-2.aws.neon.tech/db?sslmode=require",
		"postgres://u:p@ep-quiet-sky-123456.us-east-2.aws.neon.tech:5432/db":                   "postgres://u:p@ep-quiet-sky-123456.us-east-2.aws.neon.tech:5432/db",
		"postgres://u:p@db:5432/keystone?sslmode=disable":                                      "postgres://u:p@db:5432/keystone?sslmode=disable",
	}
	for in, want := range cases {
		if got := DirectFromPooled(in); got != want {
			t.Errorf("DirectFromPooled(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

func TestAnthropicKeyPrecedence(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://u:p@h/db", "JWT_SECRET": strings.Repeat("a", 32), "ANTHROPIC_MODEL": "a-model"}
	with := func(extra map[string]string) Config {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		c, err := Load(env(m))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}

	cases := []struct {
		name    string
		extra   map[string]string
		wantKey string
		enabled bool
	}{
		{"neither", nil, "", false},
		{"only keystone name", map[string]string{"KEYSTONE_ANTHROPIC_KEY": "k-keystone"}, "k-keystone", true},
		{"only fallback name", map[string]string{"ANTHROPIC_API_KEY": "k-fallback"}, "k-fallback", true},
		{"both, keystone name wins", map[string]string{"KEYSTONE_ANTHROPIC_KEY": "k-keystone", "ANTHROPIC_API_KEY": "k-fallback"}, "k-keystone", true},
		{"empty keystone name falls back", map[string]string{"KEYSTONE_ANTHROPIC_KEY": "", "ANTHROPIC_API_KEY": "k-fallback"}, "k-fallback", true},
	}
	for _, tc := range cases {
		c := with(tc.extra)
		if c.AnthropicAPIKey != tc.wantKey {
			t.Errorf("%s: key = %q, want %q", tc.name, c.AnthropicAPIKey, tc.wantKey)
		}
		if c.AnswersEnabled() != tc.enabled {
			t.Errorf("%s: AnswersEnabled = %v, want %v", tc.name, c.AnswersEnabled(), tc.enabled)
		}
	}
}

func TestPublicLinkLimitsDefaultAndNeverSwitchOff(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://u:p@h/db", "JWT_SECRET": strings.Repeat("a", 32)}
	c, err := Load(env(base))
	if err != nil {
		t.Fatal(err)
	}
	if c.AsksPerIP != 6 || c.DailyAskCap != 150 || c.ProxyHops != 1 {
		t.Errorf("defaults: %+v", c)
	}
	for _, bad := range []string{"0", "-5", "many", ""} {
		m := map[string]string{"KEYSTONE_ASKS_PER_IP": bad, "KEYSTONE_DAILY_ASK_CAP": bad, "KEYSTONE_PROXY_HOPS": bad}
		for k, v := range base {
			m[k] = v
		}
		c, _ := Load(env(m))
		if c.AsksPerIP != 6 || c.DailyAskCap != 150 || c.ProxyHops != 1 {
			t.Errorf("%q must fall back to the defaults, not switch a limit off: %+v", bad, c)
		}
	}
	base["KEYSTONE_ASKS_PER_IP"], base["KEYSTONE_DAILY_ASK_CAP"] = "30", "400"
	if c, _ := Load(env(base)); c.AsksPerIP != 30 || c.DailyAskCap != 400 {
		t.Errorf("explicit values should be used: %+v", c)
	}
}

func TestParseTeams(t *testing.T) {
	got, err := ParseTeams("Sales & Marketing:SAL; Delivery:DEL ;HR:HR;")
	if err != nil {
		t.Fatal(err)
	}
	want := []TeamSpec{{"Sales & Marketing", "SAL"}, {"Delivery", "DEL"}, {"HR", "HR"}}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("team %d: got %v want %v", i, got[i], want[i])
		}
	}
	if teams, err := ParseTeams("  "); err != nil || teams != nil {
		t.Errorf("empty input is no teams and no error, got %v, %v", teams, err)
	}
	// The prefix is what follows the last colon, so a name may contain one.
	if got, err := ParseTeams("R&D: Labs:LAB"); err != nil || got[0].Name != "R&D: Labs" {
		t.Errorf("colon in a name: %v, %v", got, err)
	}
}

func TestParseTeamsReportsEveryProblem(t *testing.T) {
	_, err := ParseTeams("NoPrefix; Ops:o ; :ABC; Eng:ENG; eng:ENX; Dev:ENG; Toolong:ABCDE")
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{`"NoPrefix" needs the form`, `"o" for "Ops"`, "no team name", `"eng" is listed twice`, `"ENG" is used by more than one`, `"ABCDE" for "Toolong"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}

func TestTeamsThresholdsModelAndWarmQuestionsFromEnv(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://u:p@h/db", "JWT_SECRET": strings.Repeat("a", 32)}
	with := func(extra map[string]string) (Config, error) {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return Load(env(m))
	}

	c, err := with(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Teams) != 0 || len(c.WarmQuestions) != 0 || c.RelevanceFloor != 0 || c.BlockMargin != 0 || c.VoyageModel != "" {
		t.Errorf("nothing is set by default: %+v", c)
	}

	c, err = with(map[string]string{
		"KEYSTONE_TEAMS":           "Support:SUP;Finance:FIN",
		"KEYSTONE_WARM_QUESTIONS":  "How do I reset my password? | What is the refund window?|| ",
		"KEYSTONE_RELEVANCE_FLOOR": "0.35",
		"KEYSTONE_BLOCK_MARGIN":    "0.08",
		"VOYAGE_MODEL":             " voyage-3.5-lite ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Teams) != 2 || c.Teams[1].Prefix != "FIN" {
		t.Errorf("teams: %v", c.Teams)
	}
	if len(c.WarmQuestions) != 2 || c.WarmQuestions[1] != "What is the refund window?" {
		t.Errorf("warm questions: %q", c.WarmQuestions)
	}
	if c.RelevanceFloor != 0.35 || c.BlockMargin != 0.08 || c.VoyageModel != "voyage-3.5-lite" {
		t.Errorf("overrides: %+v", c)
	}

	// A bad threshold is an error. It must never fall back silently.
	for _, bad := range []string{"abc", "0", "1", "-0.2", "1.5", "NaN"} {
		if _, err := with(map[string]string{"KEYSTONE_RELEVANCE_FLOOR": bad}); err == nil || !strings.Contains(err.Error(), "KEYSTONE_RELEVANCE_FLOOR") {
			t.Errorf("floor %q should be refused, got %v", bad, err)
		}
		if _, err := with(map[string]string{"KEYSTONE_BLOCK_MARGIN": bad}); err == nil || !strings.Contains(err.Error(), "KEYSTONE_BLOCK_MARGIN") {
			t.Errorf("margin %q should be refused, got %v", bad, err)
		}
	}
	if _, err := with(map[string]string{"KEYSTONE_TEAMS": "broken"}); err == nil || !strings.Contains(err.Error(), "KEYSTONE_TEAMS") {
		t.Errorf("a malformed team list should stop startup, got %v", err)
	}
}

func TestAnthropicModelIsRequiredWithAKeyAndNeverDefaulted(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://u:p@h/db", "JWT_SECRET": strings.Repeat("a", 32)}
	// No key: no model needed, and none is invented.
	c, err := Load(env(base))
	if err != nil || c.AnthropicModel != "" || c.AnswersEnabled() {
		t.Fatalf("no key should load cleanly with answers off: %+v, %v", c, err)
	}
	// A key without a model is a startup error that says what to set.
	withKey := map[string]string{"DATABASE_URL": base["DATABASE_URL"], "JWT_SECRET": base["JWT_SECRET"], "KEYSTONE_ANTHROPIC_KEY": "k"}
	if _, err := Load(env(withKey)); err == nil || !strings.Contains(err.Error(), "ANTHROPIC_MODEL") {
		t.Errorf("a key without a model must be refused, got %v", err)
	}
	withKey["ANTHROPIC_MODEL"] = " a-model "
	c, err = Load(env(withKey))
	if err != nil || c.AnthropicModel != "a-model" || !c.AnswersEnabled() {
		t.Errorf("key and model: %+v, %v", c, err)
	}
}
