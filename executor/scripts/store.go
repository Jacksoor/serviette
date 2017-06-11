package scripts

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	"golang.org/x/net/context"
)

var (
	ErrInvalidName   error = errors.New("scripts: invalid name")
	ErrAlreadyExists       = errors.New("scripts: already exists")
	ErrNotFound            = errors.New("scripts: not found")
)

type Store struct {
	rootPath string
}

func NewStore(rootPath string) (*Store, error) {
	path, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, err
	}

	return &Store{
		rootPath: path,
	}, nil
}

var nameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func (s *Store) load(ctx context.Context, ownerName string, name string) (*Script, error) {
	if !nameRegexp.MatchString(name) {
		return nil, ErrInvalidName
	}

	accountRoot := filepath.Join(s.rootPath, ownerName)
	path := filepath.Join(accountRoot, name)

	if filepath.Dir(path) != accountRoot {
		return nil, ErrInvalidName
	}

	return &Script{
		rootPath:  s.rootPath,
		ownerName: ownerName,
		name:      name,
	}, nil
}

func (s *Store) RootPath() string {
	return s.rootPath
}

func (s *Store) Create(ctx context.Context, ownerName string, name string) (*Script, error) {
	script, err := s.load(ctx, ownerName, name)
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

	accountRoot := filepath.Join(s.rootPath, ownerName)
	if err := os.MkdirAll(accountRoot, 0700); err != nil {
		return nil, err
	}

	return script, nil
}

func (s *Store) Open(ctx context.Context, ownerName string, name string) (*Script, error) {
	script, err := s.load(ctx, ownerName, name)
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

func (s *Store) AccountScripts(ctx context.Context, ownerName string) ([]*Script, error) {
	accountRoot := filepath.Join(s.rootPath, ownerName)

	infos, err := ioutil.ReadDir(accountRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	scripts := make([]*Script, len(infos))
	for i, info := range infos {
		scripts[i] = &Script{
			rootPath:  s.rootPath,
			ownerName: ownerName,
			name:      info.Name(),
		}
	}

	return scripts, nil
}
