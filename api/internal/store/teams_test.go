package store

import (
	"context"
	"strings"
	"testing"

	"keystone/internal/testutil"
)

func TestEnsureTeamsAddsMissingAndNeverRewrites(t *testing.T) {
	pool := testutil.DB(t)
	s := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DELETE FROM teams`); err != nil {
		t.Fatal(err)
	}

	if err := s.EnsureTeams(ctx, []TeamSeed{{"Support", "SUP"}, {"Finance", "FIN"}}); err != nil {
		t.Fatal(err)
	}
	// Running it again, with one more team, adds only the new one.
	if err := s.EnsureTeams(ctx, []TeamSeed{{"Support", "SUP"}, {"Finance", "FIN"}, {"Legal", "LEG"}}); err != nil {
		t.Fatal(err)
	}
	if n, err := s.CountTeams(ctx); err != nil || n != 3 {
		t.Fatalf("want 3 teams, got %d, %v", n, err)
	}

	// The same name with another prefix, and a prefix already taken, are both refused.
	if err := s.EnsureTeams(ctx, []TeamSeed{{"Support", "SPT"}}); err == nil || !strings.Contains(err.Error(), "already exists with the prefix SUP") {
		t.Errorf("changed prefix should be refused, got %v", err)
	}
	if err := s.EnsureTeams(ctx, []TeamSeed{{"Sales", "SUP"}}); err == nil || !strings.Contains(err.Error(), "already used by another team") {
		t.Errorf("taken prefix should be refused, got %v", err)
	}
	// ADM is reserved for admin accounts.
	if err := s.EnsureTeams(ctx, []TeamSeed{{"Admins", "ADM"}}); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Errorf("ADM should be reserved, got %v", err)
	}
	if n, _ := s.CountTeams(ctx); n != 3 {
		t.Errorf("refused teams must not be created, have %d", n)
	}
}
