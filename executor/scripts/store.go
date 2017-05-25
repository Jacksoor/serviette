package scripts

import (
	"encoding/base64"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrInvalidScriptName error = errors.New("invalid script name")
	ErrAlreadyExists           = errors.New("already exists")
	ErrNotFound                = errors.New("not found")
)

type Store struct {
	rootPath string
}

func NewStore(rootPath string) *Store {
	return &Store{
		rootPath: rootPath,
	}
}

func (s *Store) load(accountHandle []byte, name string) (*Script, error) {
	if strings.Contains(name, " ") {
		return nil, ErrInvalidScriptName
	}

	accountRoot := filepath.Join(s.rootPath, base64.RawURLEncoding.EncodeToString(accountHandle))
	path := filepath.Join(accountRoot, name)

	if filepath.Dir(path) != accountRoot {
		return nil, ErrInvalidScriptName
	}

	return &Script{
		rootPath:      s.rootPath,
		accountHandle: accountHandle,
		name:          name,
	}, nil
}

func (s *Store) Create(accountHandle []byte, name string) (*Script, error) {
	script, err := s.load(accountHandle, name)
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

func (s *Store) Open(accountHandle []byte, name string) (*Script, error) {
	script, err := s.load(accountHandle, name)
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

func (s *Store) AccountScripts(accountHandle []byte) ([]*Script, error) {
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
