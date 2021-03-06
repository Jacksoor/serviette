package accounts

import (
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"syscall"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/context"

	accountspb "github.com/porpoises/kobun4/executor/accountsservice/v1pb"
)

var nameRegexp = regexp.MustCompile(`^[a-z0-9_-]{1,20}$`)

var (
	ErrNotFound        error = errors.New("accounts: not found")
	ErrInvalidName           = errors.New("accounts: invalid name")
	ErrAlreadyExists         = errors.New("accounts: already exists")
	ErrUnauthenticated       = errors.New("accounts: unauthenticated")
)

type Store struct {
	db              *sql.DB
	storageRootPath string
	makestoragePath string
}

func (s *Store) StorageRootPath() string {
	return s.storageRootPath
}

func NewStore(db *sql.DB, storageRootPath string, makestoragePath string) *Store {
	return &Store{
		db:              db,
		storageRootPath: storageRootPath,
		makestoragePath: makestoragePath,
	}
}

type Account struct {
	db              *sql.DB
	storageRootPath string
	Name            string
}

func getStorageUsage(path string) (*accountspb.StorageUsage, error) {
	var statfsBuf syscall.Statfs_t
	if err := syscall.Statfs(path, &statfsBuf); err != nil {
		return nil, err
	}
	return &accountspb.StorageUsage{
		TotalSize: uint64(statfsBuf.Bsize) * statfsBuf.Blocks,
		FreeSize:  uint64(statfsBuf.Bsize) * statfsBuf.Bavail,
	}, nil
}

func (a *Account) Traits(ctx context.Context) (*accountspb.Traits, error) {
	traits := &accountspb.Traits{}

	if err := a.db.QueryRowContext(ctx, `
		select time_limit_seconds,
		       memory_limit,
		       tmpfs_size,
		       allow_network_access,
		       blkio_weight,
		       cpu_shares,
		       allowed_services,
		       allowed_output_formats,
		       max_messages_per_invocation
		from accounts
		where name = $1
	`, a.Name).Scan(
		&traits.TimeLimitSeconds,
		&traits.MemoryLimit,
		&traits.TmpfsSize,
		&traits.AllowNetworkAccess,
		&traits.BlkioWeight,
		&traits.CpuShares,
		pq.Array(&traits.AllowedService),
		pq.Array(&traits.AllowedOutputFormat),
		&traits.MaxMessagesPerInvocation,
	); err != nil {
		return nil, err
	}

	return traits, nil
}

func (a *Account) StoragePath() string {
	return filepath.Join(a.storageRootPath, a.Name)
}

func (a *Account) ScriptsStoragePath() string {
	return filepath.Join(a.StoragePath(), "scripts")
}

func (a *Account) PrivateStoragePath() string {
	return filepath.Join(a.StoragePath(), "private")
}

func (a *Account) ScriptsStorageUsage() (*accountspb.StorageUsage, error) {
	return getStorageUsage(a.ScriptsStoragePath())
}

func (a *Account) PrivateStorageUsage() (*accountspb.StorageUsage, error) {
	return getStorageUsage(a.PrivateStoragePath())
}

func (a *Account) Authenticate(ctx context.Context, password string) error {
	var pwhash string
	if err := a.db.QueryRowContext(ctx, `
		select password_hash
		from accounts
		where name = $1
	`, a.Name).Scan(&pwhash); err != nil {
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(pwhash), []byte(password)); err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return ErrUnauthenticated
		}
	}

	return nil
}

func (a *Account) SetPassword(ctx context.Context, password string) error {
	pwhash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if _, err := a.db.ExecContext(ctx, `
		update accounts
		set password_hash = $1
		where name = $2
	`, string(pwhash), a.Name); err != nil {
		return err
	}

	return nil
}

func (s *Store) destroyStorage(username string) error {
	cmd := exec.Command(s.makestoragePath, "-name="+username, "-destroy")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s *Store) makeStorage(username string) error {
	cmd := exec.Command(s.makestoragePath, "-name="+username)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s *Store) Create(ctx context.Context, username string, password string, identifiers []string) error {
	if !nameRegexp.MatchString(username) {
		return ErrInvalidName
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	pwhash := []byte{}
	if password != "" {
		pwhash, err = bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		insert into accounts (name, password_hash)
		values ($1, $2)
	`, username, string(pwhash)); err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" /* unique_violation */ {
			return ErrAlreadyExists
		}
		return err
	}

	for _, identifier := range identifiers {
		if _, err := tx.ExecContext(ctx, `
			insert into account_identifiers (account_name, identifier)
			values ($1, $2)
		`, username, identifier); err != nil {
			if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" /* unique_violation */ {
				return ErrAlreadyExists
			}
			return err
		}
	}

	// Make sure storage is clear.
	if _, err := os.Stat(filepath.Join(s.storageRootPath, username)); err == nil {
		if err := s.destroyStorage(username); err != nil {
			return err
		}
	}

	if err := s.makeStorage(username); err != nil {
		return err
	}

	tx.Commit()
	return nil
}

func (s *Store) Account(ctx context.Context, name string) (*Account, error) {
	account := &Account{
		db:              s.db,
		storageRootPath: s.storageRootPath,
		Name:            name,
	}

	var count int

	if err := s.db.QueryRowContext(ctx, `
		select count(1)
		from accounts
		where name = $1
	`, name).Scan(&count); err != nil {
		return nil, err
	}

	if count == 0 {
		return nil, ErrNotFound
	}

	return account, nil
}

func (s *Store) Accounts(ctx context.Context, offset, limit uint32) ([]*Account, error) {
	accounts := make([]*Account, 0)

	rows, err := s.db.QueryContext(ctx, `
		select name from accounts
		offset $1 limit $2
	`, offset, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}

		accounts = append(accounts, &Account{
			db:              s.db,
			storageRootPath: s.storageRootPath,
			Name:            name,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return accounts, nil
}

func (s *Store) AccountsByIdentifier(ctx context.Context, identifier string) ([]*Account, error) {
	accounts := make([]*Account, 0)

	rows, err := s.db.QueryContext(ctx, `
		select account_name from account_identifiers
		where identifier = $1
	`, identifier)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}

		accounts = append(accounts, &Account{
			db:              s.db,
			storageRootPath: s.storageRootPath,
			Name:            name,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return accounts, nil
}

func (s *Store) CheckAccountIdentifier(ctx context.Context, username string, identifier string) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `
		select count(1) from account_identifiers
		where account_name = $1 and identifier = $2
	`, username, identifier).Scan(&count); err != nil {
		return err
	}

	if count == 0 {
		return ErrNotFound
	}

	return nil
}
