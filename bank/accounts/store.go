package accounts

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"golang.org/x/net/context"
)

var (
	ErrNotFound error = errors.New("not found")
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{
		db: db,
	}
}

func (s *Store) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return s.db.BeginTx(ctx, nil)
}

func (s *Store) Load(ctx context.Context, tx *sql.Tx, handle []byte) (*Account, error) {
	var key []byte
	if err := tx.QueryRowContext(ctx, `
		select key from accounts
		where handle = ?
	`, handle).Scan(&key); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &Account{
		handle: handle,
		key:    key,
	}, nil
}

func (s *Store) Create(ctx context.Context, tx *sql.Tx) (*Account, error) {
	handle := make([]byte, 128/8)
	if _, err := rand.Read(handle); err != nil {
		return nil, err
	}

	key := make([]byte, 128/8)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		insert into accounts (handle, key)
		values (?, ?)
	`, handle, key); err != nil {
		return nil, err
	}

	return &Account{
		handle: handle,
		key:    key,
	}, nil
}

func (s *Store) Accounts(ctx context.Context, tx *sql.Tx) ([]*Account, error) {
	accounts := make([]*Account, 0)

	rows, err := tx.QueryContext(ctx, `
		select handle, key from accounts
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var handle []byte
		var key []byte

		if err := rows.Scan(&handle, &key); err != nil {
			return nil, err
		}

		accounts = append(accounts, &Account{
			handle: handle,
			key:    key,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return accounts, nil
}

func (s *Store) LoadByAlias(ctx context.Context, tx *sql.Tx, name string) (*Account, error) {
	var handle []byte
	var key []byte

	if err := tx.QueryRowContext(ctx, `
		select handle, key from accounts
		inner join aliases on aliases.account_handle = accounts.handle
		where aliases.name = ?
	`, name).Scan(&handle, &key); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &Account{
		handle: handle,
		key:    key,
	}, nil
}

func (s *Store) SetAlias(ctx context.Context, tx *sql.Tx, name string, account *Account) error {
	var r sql.Result
	var err error

	if account == nil {
		r, err = tx.ExecContext(ctx, `
			delete from aliases
			where name = ?
		`, name)
	} else {
		r, err = tx.ExecContext(ctx, `
			insert or replace into aliases (name, account_handle)
			values (?, ?)
		`, name, account.Handle())
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
