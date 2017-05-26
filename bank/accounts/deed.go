package accounts

import (
	"database/sql"
	"golang.org/x/net/context"

	deedspb "github.com/porpoises/kobun4/bank/deedsservice/v1pb"
)

type Deed struct {
	id int64
}

func (d *Deed) Info(ctx context.Context, tx *sql.Tx) (*deedspb.Info, error) {
	info := &deedspb.Info{}

	if err := tx.QueryRowContext(ctx, `
		select deed_types.name, deeds.name, owner_account_handle, expiry_time_unix from deeds
		inner join deed_types on deed_types.id = deeds.deed_type_id
		where deeds.id = ?
	`, d.id).Scan(&info.Type, &info.Name, &info.OwnerAccountHandle, &info.ExpiryTimeUnix); err != nil {
		return nil, err
	}

	return info, nil
}

func (d *Deed) Content(ctx context.Context, tx *sql.Tx) ([]byte, error) {
	var content []byte

	if err := tx.QueryRowContext(ctx, `
		select content from deeds
		where id = ?
	`, d.id).Scan(&content); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return content, nil
}

func (d *Deed) Delete(ctx context.Context, tx *sql.Tx) error {
	r, err := tx.ExecContext(ctx, `
		delete from deeds
		where id = ?
	`, d.id)
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

func (d *Deed) Renew(ctx context.Context, tx *sql.Tx, periods int64) error {
	r, err := tx.ExecContext(ctx, `
		update deeds
		set expiry_time_unix = expiry_time_unix + (select duration_seconds * ? from deed_types where id = deeds.deed_type_id)
		where id = ?
	`, periods, d.id)
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

func (d *Deed) Update(ctx context.Context, tx *sql.Tx, content []byte) error {
	r, err := tx.ExecContext(ctx, `
		update deeds
		set content = ?
		where id = ?
	`, content, d.id)
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
