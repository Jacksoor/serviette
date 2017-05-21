package accounts

import (
	"database/sql"
	"golang.org/x/net/context"
)

type Account struct {
	handle []byte
	key    []byte
}

func (a *Account) Handle() []byte {
	return a.handle
}

func (a *Account) Key() []byte {
	return a.key
}

func (a *Account) Balance(ctx context.Context, tx *sql.Tx) (int64, error) {
	var balance int64

	if err := tx.QueryRowContext(ctx, `
		select balance from accounts
		where handle = ?
	`, a.handle).Scan(&balance); err != nil {
		if err == sql.ErrNoRows {
			return 0, ErrNotFound
		}
		return 0, err
	}

	return balance, nil
}

func (a *Account) AddMoney(ctx context.Context, tx *sql.Tx, amount int64) error {
	r, err := tx.ExecContext(ctx, `
		update accounts
		set balance = balance + ?
		where handle = ?
	`, amount, a.handle)
	if err != nil {
		return err
	}

	n, err := r.RowsAffected()
	if err != nil {
		return err
	}

	if n != 1 {
		return ErrNotFound
	}

	return nil
}

func (a *Account) Delete(ctx context.Context, tx *sql.Tx) error {
	r, err := tx.ExecContext(ctx, `
		delete from accounts
		where handle = ?
	`, a.handle)
	if err != nil {
		return err
	}

	n, err := r.RowsAffected()
	if err != nil {
		return err
	}

	if n != 1 {
		return ErrNotFound
	}

	return nil
}
