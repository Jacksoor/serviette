package store

import (
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

func New(db *sql.DB) *Store {
	return &Store{
		db: db,
	}
}

type Account struct {
	Handle []byte
	Key    []byte
}

func (s *Store) Accounts(ctx context.Context, userID string) (map[string]*Account, error) {
	accounts := make(map[string]*Account, 0)

	rows, err := s.db.QueryContext(ctx, `
		select name, account_handle, account_key from associations
		where user_id = ?
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var name string

		account := &Account{}
		if err := rows.Scan(&name, &account.Handle, &account.Key); err != nil {
			return nil, err
		}
		accounts[name] = account
	}

	return accounts, nil
}

func (s *Store) Account(ctx context.Context, userID string, name string) (*Account, error) {
	account := &Account{}

	if err := s.db.QueryRowContext(ctx, `
		select account_handle, account_key from associations
		where user_id = ?
	`, userID).Scan(&account.Handle, &account.Key); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return account, nil
}

func (s *Store) Associate(ctx context.Context, userID string, name string, accountHandle []byte, accountKey []byte) error {
	if _, err := s.db.ExecContext(ctx, `
		insert or replace into associations (user_id, name, account_handle, account_key)
		values (?, ?, ?, ?)
	`, userID, name, accountHandle, accountKey); err != nil {
		return err
	}

	return nil
}
