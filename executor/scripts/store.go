package scripts

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

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

func (s *Store) RootPath() string {
	return s.rootPath
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
