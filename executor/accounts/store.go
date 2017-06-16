package accounts

import (
	"database/sql"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/context"
)

var (
	ErrNotFound        error = errors.New("accounts: not found")
	ErrUnauthenticated       = errors.New("accounts: unauthenticated")
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{
		db: db,
	}
}

type Account struct {
	PasswordHash []byte
	TimeLimit    time.Duration
	MemoryLimit  int64
	TmpfsSize    int64

	AllowMessagingService bool
	AllowRawOutput        bool
	AllowNetworkAccess    bool
}

func (s *Store) Account(ctx context.Context, name string) (*Account, error) {
	account := &Account{}

	var timeLimitSeconds int64

	if err := s.db.QueryRowContext(ctx, `
		select password_hash, time_limit_seconds, memory_limit, tmpfs_size, allow_messaging_service, allow_raw_output, allow_network_access
		from accounts
		where name = ?
	`, name).Scan(&account.PasswordHash, &timeLimitSeconds, &account.MemoryLimit, &account.TmpfsSize, &account.AllowMessagingService, &account.AllowRawOutput, &account.AllowNetworkAccess); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	account.TimeLimit = time.Duration(timeLimitSeconds) * time.Second

	return account, nil
}

func (s *Store) AccountNames(ctx context.Context) ([]string, error) {
	names := make([]string, 0)

	rows, err := s.db.QueryContext(ctx, `
		select name from accounts
	`)
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

func (s *Store) Authenticate(ctx context.Context, userName string, password string) error {
	account, err := s.Account(ctx, userName)
	if err != nil {
		return err
	}

	if err := bcrypt.CompareHashAndPassword(account.PasswordHash, []byte(password)); err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return ErrUnauthenticated
		}
	}

	return nil
}
