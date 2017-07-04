package accounts

import (
	"database/sql"
	"errors"
	"path/filepath"
	"syscall"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/context"

	accountspb "github.com/porpoises/kobun4/executor/accountsservice/v1pb"
)

var (
	ErrNotFound        error = errors.New("accounts: not found")
	ErrUnauthenticated       = errors.New("accounts: unauthenticated")
)

type Store struct {
	db              *sql.DB
	storageRootPath string
}

func NewStore(db *sql.DB, storageRootPath string) *Store {
	return &Store{
		db:              db,
		storageRootPath: storageRootPath,
	}
}

type Account struct {
	storageRootPath string

	Name string

	PasswordHash []byte

	Traits *accountspb.Traits
}

func (a *Account) IsOutputFormatAllowed(format string) bool {
	for _, allowedFormat := range a.Traits.AllowedOutputFormat {
		if allowedFormat == format {
			return true
		}
	}
	return false
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
	if err := bcrypt.CompareHashAndPassword(a.PasswordHash, []byte(password)); err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return ErrUnauthenticated
		}
	}

	return nil
}

func (s *Store) Account(ctx context.Context, name string) (*Account, error) {
	account := &Account{
		storageRootPath: s.storageRootPath,

		Traits: &accountspb.Traits{},
	}

	if err := s.db.QueryRowContext(ctx, `
		select name,
		       password_hash,
		       time_limit_seconds,
		       memory_limit,
		       tmpfs_size,
		       allow_network_access,
		       allowed_services,
		       allowed_output_formats
		from accounts
		where name = $1
	`, name).Scan(
		&account.Name,
		&account.PasswordHash,
		&account.Traits.TimeLimitSeconds,
		&account.Traits.MemoryLimit,
		&account.Traits.TmpfsSize,
		&account.Traits.AllowNetworkAccess,
		pq.Array(&account.Traits.AllowedService),
		pq.Array(&account.Traits.AllowedOutputFormat),
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return account, nil
}

func (s *Store) AccountNames(ctx context.Context, offset, limit uint32) ([]string, error) {
	names := make([]string, 0)

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

		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return names, nil
}
