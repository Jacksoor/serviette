package scripts

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"golang.org/x/net/context"
)

var (
	ErrInvalidName   error = errors.New("invalid name")
	ErrAlreadyExists       = errors.New("already exists")
	ErrNotFound            = errors.New("not found")
)

type Store struct {
	rootPath string
	db       *sql.DB
}

func NewStore(rootPath string, db *sql.DB) *Store {
	return &Store{
		rootPath: rootPath,
		db:       db,
	}
}

var nameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func (s *Store) load(ctx context.Context, accountHandle []byte, name string) (*Script, error) {
	if !nameRegexp.MatchString(name) {
		return nil, ErrInvalidName
	}

	accountRoot := filepath.Join(s.rootPath, base64.RawURLEncoding.EncodeToString(accountHandle))
	path := filepath.Join(accountRoot, name)

	if filepath.Dir(path) != accountRoot {
		return nil, ErrInvalidName
	}

	return &Script{
		rootPath:      s.rootPath,
		accountHandle: accountHandle,
		name:          name,
	}, nil
}

func (s *Store) Create(ctx context.Context, accountHandle []byte, name string) (*Script, error) {
	script, err := s.load(ctx, accountHandle, name)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(script.Path()); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		return nil, ErrAlreadyExists
	}

	accountRoot := filepath.Join(s.rootPath, base64.RawURLEncoding.EncodeToString(accountHandle))
	if err := os.MkdirAll(accountRoot, 0700); err != nil {
		return nil, err
	}

	return script, nil
}

func (s *Store) Open(ctx context.Context, accountHandle []byte, name string) (*Script, error) {
	script, err := s.load(ctx, accountHandle, name)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(script.Path()); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return script, nil
}

func (s *Store) AccountScripts(ctx context.Context, accountHandle []byte) ([]*Script, error) {
	accountRoot := filepath.Join(s.rootPath, base64.RawURLEncoding.EncodeToString(accountHandle))

	infos, err := ioutil.ReadDir(accountRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	scripts := make([]*Script, len(infos))
	for i, info := range infos {
		scripts[i] = &Script{
			rootPath:      s.rootPath,
			accountHandle: accountHandle,
			name:          info.Name(),
		}
	}

	return scripts, nil
}

type Alias struct {
	Name          string
	AccountHandle []byte
	ScriptName    string
	ExpiryTime    time.Time
}

func (s *Store) expireAliases(ctx context.Context) error {
	now := time.Now()

	_, err := s.db.ExecContext(ctx, `
		delete from aliases
		where expiry_time_unix <= ?
	`, now.Unix())

	return err
}

func (s *Store) LoadAlias(ctx context.Context, name string) (*Alias, error) {
	if err := s.expireAliases(ctx); err != nil {
		return nil, err
	}

	alias := &Alias{
		Name: name,
	}

	var expiryTimeUnix int64

	if err := s.db.QueryRowContext(ctx, `
		select account_handle, script_name, expiry_time_unix from aliases
		where aliases.name = ?
	`, name).Scan(&alias.AccountHandle, &alias.ScriptName, &expiryTimeUnix); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	alias.ExpiryTime = time.Unix(expiryTimeUnix, 0)

	return alias, nil
}

func (s *Store) SetAlias(ctx context.Context, name string, accountHandle []byte, scriptName string, expiry time.Time) error {
	if !nameRegexp.MatchString(name) {
		return ErrInvalidName
	}

	if err := s.expireAliases(ctx); err != nil {
		return nil
	}

	var r sql.Result
	var err error

	if accountHandle == nil {
		r, err = s.db.ExecContext(ctx, `
			delete from aliases
			where name = ?
		`, name)
	} else {
		r, err = s.db.ExecContext(ctx, `
			insert or replace into aliases (name, account_handle, script_name, expiry_time_unix)
			values (?, ?, ?, ?)
		`, name, accountHandle, scriptName, expiry.Unix())
	}

	if err != nil {
		return err
	}

	n, err := r.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *Store) Aliases(ctx context.Context) ([]*Alias, error) {
	if err := s.expireAliases(ctx); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		select name, account_handle, script_name, expiry_time_unix
		from aliases
	`)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aliases := make([]*Alias, 0)

	for rows.Next() {
		alias := &Alias{}
		var expiryTimeUnix int64

		if err := rows.Scan(&alias.Name, &alias.AccountHandle, &alias.ScriptName, &expiryTimeUnix); err != nil {
			return nil, err
		}
		alias.ExpiryTime = time.Unix(expiryTimeUnix, 0)
		aliases = append(aliases, alias)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return aliases, nil
}
