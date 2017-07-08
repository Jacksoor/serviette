package scripts

import (
	"database/sql"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"golang.org/x/net/context"

	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	ErrInvalid error = errors.New("invalid")
)

type Script struct {
	db              *sql.DB
	storageRootPath string

	OwnerName string
	Name      string
}

func (s *Script) QualifiedName() string {
	return filepath.Join(s.OwnerName, s.Name)
}

func (s *Script) Path() string {
	return filepath.Join(s.storageRootPath, s.OwnerName, "scripts", s.Name)
}

func (s *Script) Content(ctx context.Context) ([]byte, error) {
	return ioutil.ReadFile(s.Path())
}

func (s *Script) SetContent(ctx context.Context, content []byte) error {
	return ioutil.WriteFile(s.Path(), content, 0755)
}

func (s *Script) Meta(ctx context.Context) (*scriptspb.Meta, error) {
	meta := &scriptspb.Meta{}

	if err := s.db.QueryRowContext(ctx, `
		select description, visibility
		from scripts
		where owner_name = $1 and
		      script_name = $2
	`, s.OwnerName, s.Name).Scan(&meta.Description, &meta.Visibility); err != nil {
		return nil, err
	}

	return meta, nil
}

func (s *Script) SetMeta(ctx context.Context, meta *scriptspb.Meta) error {
	if meta.Visibility > scriptspb.Visibility_PUBLISHED {
		return ErrInvalid
	}

	if _, err := s.db.ExecContext(ctx, `
		update scripts
		set description = $1,
		    visibility = $2
		where owner_name = $3 and
		      script_name = $4
	`, meta.Description, meta.Visibility, s.OwnerName, s.Name); err != nil {
		return err
	}
	return nil
}

func (s *Script) Delete(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		delete from scripts
		where owner_name = $1 and
		      script_name = $2
	`, s.OwnerName, s.Name); err != nil {
		return err
	}

	if err := os.Remove(s.Path()); err != nil {
		return nil
	}

	tx.Commit()
	return nil
}
