package accounts

import (
	"database/sql"
	"golang.org/x/net/context"

	namespb "github.com/porpoises/kobun4/bank/namesservice/v1pb"
)

type Name struct {
	id int64
}

func (a *Name) Info(ctx context.Context, tx *sql.Tx) (*namespb.Info, error) {
	info := &namespb.Info{}

	if err := tx.QueryRowContext(ctx, `
		select name_type.name, name, owner_account_handle, expiry_time_unix from names
		inner join name_type on name_type.id = names.name_type_id
		where names.id = ?
	`, a.id).Scan(&info.Type, &info.Name, &info.OwnerAccountHandle, &info.ExpiryTimeUnix); err != nil {
		return nil, err
	}

	return info, nil
}

func (a *Name) Content(ctx context.Context, tx *sql.Tx) ([]byte, error) {
	var content []byte

	if err := tx.QueryRowContext(ctx, `
		select content from names
		where id = ?
	`, a.id).Scan(&content); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return content, nil
}

func (a *Name) Delete(ctx context.Context, tx *sql.Tx) error {
	r, err := tx.ExecContext(ctx, `
		delete from names
		where id = ?
	`, a.id)
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

func (a *Name) Renew(ctx context.Context, tx *sql.Tx, periods int64) error {
	r, err := tx.ExecContext(ctx, `
		update names
		set expiry_time_unix = expiry_time_unix + (select duration_seconds * ? from name_types where id = names.name_type_id)
		where id = ?
	`, periods, a.id)
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
