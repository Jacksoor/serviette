package scripts

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"regexp"

	"github.com/lib/pq"
	"golang.org/x/net/context"
)

var (
	ErrInvalidName   error = errors.New("scripts: invalid name")
	ErrAlreadyExists       = errors.New("scripts: already exists")
	ErrNotFound            = errors.New("scripts: not found")
)

type Store struct {
	db       *sql.DB
	rootPath string
}

func NewStore(db *sql.DB, rootPath string) (*Store, error) {
	path, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, err
	}

	return &Store{
		db:       db,
		rootPath: path,
	}, nil
}

var nameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func (s *Store) RootPath() string {
	return s.rootPath
}

func (s *Store) Create(ctx context.Context, ownerName string, name string) (*Script, error) {
	if !nameRegexp.MatchString(ownerName) || !nameRegexp.MatchString(name) {
		return nil, ErrInvalidName
	}

	accountRoot := filepath.Join(s.rootPath, ownerName)
	path := filepath.Join(accountRoot, name)

	if filepath.Dir(path) != accountRoot {
		return nil, ErrInvalidName
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		insert into scripts (owner_name, script_name)
		values ($1, $2)
	`, ownerName, name); err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" /* unique_violation */ {
			return nil, ErrAlreadyExists
		}
		return nil, err
	}

	script := &Script{
		db:       s.db,
		rootPath: s.rootPath,

		OwnerName: ownerName,
		Name:      name,
	}

	if _, err := os.Stat(script.Path()); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		return nil, errors.New("script doesn't exist in db but exists on disk")
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return script, nil
}

func (s *Store) Open(ctx context.Context, ownerName string, name string) (*Script, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `
		select count(1)
		from scripts
		where owner_name = $1 and
		      script_name = $2
	`, ownerName, name).Scan(&count); err != nil {
		return nil, err
	}

	if count == 0 {
		return nil, ErrNotFound
	}

	return &Script{
		db:       s.db,
		rootPath: s.rootPath,

		OwnerName: ownerName,
		Name:      name,
	}, nil
}

func (s *Store) PublishedScripts(ctx context.Context) ([]*Script, error) {
	scripts := make([]*Script, 0)

	rows, err := s.db.QueryContext(ctx, `
		select owner_name, script_name
		from scripts
		where published
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		script := &Script{
			db:       s.db,
			rootPath: s.rootPath,
		}
		if err := rows.Scan(&script.OwnerName, &script.Name); err != nil {
			return nil, err
		}
		scripts = append(scripts, script)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return scripts, nil
}

func (s *Store) Scripts(ctx context.Context, ownerName string, query string, viewerName string, offset, limit uint32) ([]*Script, error) {
	scripts := make([]*Script, 0)

	rows, err := s.db.QueryContext(ctx, `
		select owner_name, script_name
		from scripts, plainto_tsquery('english', $2) tsq
		where ($1 = '' or owner_name = $1) and
		      ($2 = '' or (to_tsvector('english', script_name || ' ' || description) @@ tsq)) and
		      (owner_name = $3 or published)
		order by ts_rank_cd(to_tsvector('english', script_name || ' ' || description), tsq) desc, owner_name asc, script_name asc
		offset $4 limit $5
	`, ownerName, query, viewerName, offset, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		script := &Script{
			db:       s.db,
			rootPath: s.rootPath,
		}
		if err := rows.Scan(&script.OwnerName, &script.Name); err != nil {
			return nil, err
		}
		scripts = append(scripts, script)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return scripts, nil
}
