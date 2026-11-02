// Package store holds the SQL for everything except retrieval. Reading chunk text
// or embeddings for answering lives in package retrieve, and nowhere else.
package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadySetUp  = errors.New("already set up")
	ErrNumberTaken   = errors.New("employee number taken")
	ErrUnknownTeam   = errors.New("unknown team")
	ErrInvalidNumber = errors.New("invalid employee number")
)

const AdminPrefix = "ADM"

var employeeNumberRE = regexp.MustCompile(`^[A-Z]{2,4}-[0-9]{3,6}$`)

type Store struct {
	pool       *pgxpool.Pool
	maxLogRows int // 0 means DefaultMaxQueryLogRows
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

type Employee struct {
	ID           int64
	Number       string
	Name         string
	TeamID       *int64
	TeamName     *string
	Role         string
	PasswordHash string
	CreatedAt    time.Time
}

type Team struct {
	ID           int64
	Name         string
	NumberPrefix string
}

type TeamWithMembers struct {
	Team
	Members []Employee
}

const employeeSelect = `
	SELECT e.id, e.employee_number, e.name, e.team_id, t.name, e.role, e.password_hash, e.created_at
	FROM employees e LEFT JOIN teams t ON t.id = e.team_id`

type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func scanEmployee(row pgx.Row) (Employee, error) {
	var e Employee
	err := row.Scan(&e.ID, &e.Number, &e.Name, &e.TeamID, &e.TeamName, &e.Role, &e.PasswordHash, &e.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Employee{}, ErrNotFound
	}
	return e, err
}

func employeeByID(ctx context.Context, q rowQuerier, id int64) (Employee, error) {
	return scanEmployee(q.QueryRow(ctx, employeeSelect+` WHERE e.id = $1`, id))
}

func (s *Store) EmployeeByID(ctx context.Context, id int64) (Employee, error) {
	return employeeByID(ctx, s.pool, id)
}

func (s *Store) EmployeeByNumber(ctx context.Context, number string) (Employee, error) {
	return scanEmployee(s.pool.QueryRow(ctx, employeeSelect+` WHERE e.employee_number = $1`, NormalizeNumber(number)))
}

func NormalizeNumber(n string) string { return strings.ToUpper(strings.TrimSpace(n)) }

func (s *Store) NeedsSetup(ctx context.Context) (bool, error) {
	var any bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM employees)`).Scan(&any)
	return !any, err
}

// CreateFirstAdmin creates ADM-001, but only while no employee exists. The table
// lock makes two simultaneous setups safe: the second waits, then sees a row.
func (s *Store) CreateFirstAdmin(ctx context.Context, name, passwordHash string) (Employee, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Employee{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	if _, err := tx.Exec(ctx, `LOCK TABLE employees IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return Employee{}, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM employees)`).Scan(&exists); err != nil {
		return Employee{}, err
	}
	if exists {
		return Employee{}, ErrAlreadySetUp
	}
	var id int64
	err = tx.QueryRow(ctx,
		`INSERT INTO employees (employee_number, name, team_id, role, password_hash)
		 VALUES ($1, $2, NULL, 'admin', $3) RETURNING id`,
		AdminPrefix+"-001", strings.TrimSpace(name), passwordHash).Scan(&id)
	if err != nil {
		return Employee{}, err
	}
	e, err := employeeByID(ctx, tx, id)
	if err != nil {
		return Employee{}, err
	}
	return e, tx.Commit(ctx)
}

type NewEmployee struct {
	Name         string
	TeamID       int64
	Number       string // optional; assigned from the team prefix when empty
	PasswordHash string
}

// CreateEmployee adds a team member. With no number given it assigns the next one
// for the team, such as DEL-002, retrying if a concurrent insert took it.
func (s *Store) CreateEmployee(ctx context.Context, in NewEmployee) (Employee, error) {
	var prefix string
	err := s.pool.QueryRow(ctx, `SELECT number_prefix FROM teams WHERE id = $1`, in.TeamID).Scan(&prefix)
	if errors.Is(err, pgx.ErrNoRows) {
		return Employee{}, ErrUnknownTeam
	}
	if err != nil {
		return Employee{}, err
	}

	number := NormalizeNumber(in.Number)
	if number != "" && !employeeNumberRE.MatchString(number) {
		return Employee{}, ErrInvalidNumber
	}

	for attempt := 0; attempt < 3; attempt++ {
		n := number
		if n == "" {
			if n, err = s.nextNumber(ctx, prefix); err != nil {
				return Employee{}, err
			}
		}
		var id int64
		err = s.pool.QueryRow(ctx,
			`INSERT INTO employees (employee_number, name, team_id, role, password_hash)
			 VALUES ($1, $2, $3, 'employee', $4) RETURNING id`,
			n, strings.TrimSpace(in.Name), in.TeamID, in.PasswordHash).Scan(&id)
		if err == nil {
			return s.EmployeeByID(ctx, id)
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if number != "" {
				return Employee{}, ErrNumberTaken
			}
			continue // an auto-assigned number was taken a moment ago; pick the next
		}
		return Employee{}, err
	}
	return Employee{}, fmt.Errorf("could not assign an employee number for prefix %s", prefix)
}

func (s *Store) nextNumber(ctx context.Context, prefix string) (string, error) {
	var next int
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(substring(employee_number FROM '-([0-9]+)$')::int), 0) + 1
		 FROM employees WHERE employee_number LIKE $1`, prefix+"-%").Scan(&next)
	return fmt.Sprintf("%s-%03d", prefix, next), err
}

// TeamSeed is one team to make sure exists.
type TeamSeed struct {
	Name   string
	Prefix string
}

// EnsureTeams creates any team that does not exist yet. It never renames, reorders or removes
// one, so restarting with a longer list only adds. A name that exists with a different prefix,
// or a prefix already used by another team, is an error and not a silent skip: existing
// employee numbers carry the old prefix.
func (s *Store) EnsureTeams(ctx context.Context, seeds []TeamSeed) error {
	for _, t := range seeds {
		if t.Prefix == AdminPrefix {
			return fmt.Errorf("team %q: the prefix %s is reserved for admin accounts", t.Name, AdminPrefix)
		}
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO teams (name, number_prefix) VALUES ($1, $2) ON CONFLICT DO NOTHING`, t.Name, t.Prefix); err != nil {
			return err
		}
		var prefix string
		err := s.pool.QueryRow(ctx, `SELECT number_prefix FROM teams WHERE name = $1`, t.Name).Scan(&prefix)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("team %q: the prefix %s is already used by another team", t.Name, t.Prefix)
		case err != nil:
			return err
		case prefix != t.Prefix:
			return fmt.Errorf("team %q already exists with the prefix %s, not %s; prefixes cannot be changed once created", t.Name, prefix, t.Prefix)
		}
	}
	return nil
}

// CountTeams is how many teams exist.
func (s *Store) CountTeams(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM teams`).Scan(&n)
	return n, err
}

// TeamsWithMembers lists every team with its employees, ordered by employee number.
func (s *Store) TeamsWithMembers(ctx context.Context) ([]TeamWithMembers, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, number_prefix FROM teams ORDER BY id`)
	if err != nil {
		return nil, err
	}
	teams, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (TeamWithMembers, error) {
		var t TeamWithMembers
		return t, r.Scan(&t.ID, &t.Name, &t.NumberPrefix)
	})
	if err != nil {
		return nil, err
	}

	erows, err := s.pool.Query(ctx, employeeSelect+` WHERE e.role = 'employee' ORDER BY e.employee_number`)
	if err != nil {
		return nil, err
	}
	members, err := pgx.CollectRows(erows, func(r pgx.CollectableRow) (Employee, error) { return scanEmployee(r) })
	if err != nil {
		return nil, err
	}
	byTeam := map[int64]int{}
	for i, t := range teams {
		byTeam[t.ID] = i
	}
	for _, m := range members {
		if m.TeamID != nil {
			i := byTeam[*m.TeamID]
			teams[i].Members = append(teams[i].Members, m)
		}
	}
	return teams, nil
}
