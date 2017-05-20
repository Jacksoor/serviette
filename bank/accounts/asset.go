package accounts

import (
	"database/sql"
	"golang.org/x/net/context"

	assetspb "github.com/porpoises/kobun4/bank/assetsservice/v1pb"
)

type Asset struct {
	id int64
}

func (a *Asset) Info(ctx context.Context, tx *sql.Tx) (*assetspb.Info, error) {
	info := &assetspb.Info{}

	if err := tx.QueryRowContext(ctx, `
		select asset_type.name, name, expiry_time_unix from assets
		inner join asset_type on asset_type.id = assets.asset_type_id
		where assets.id = ?
	`, a.id).Scan(&info.Type, &info.Name, &info.ExpiryTimeUnix); err != nil {
		return nil, err
	}

	return info, nil
}

func (a *Asset) Content(ctx context.Context, tx *sql.Tx) ([]byte, error) {
	var content []byte

	if err := tx.QueryRowContext(ctx, `
		select content from assets
		where id = ?
	`, a.id).Scan(&content); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return content, nil
}

func (a *Asset) Delete(ctx context.Context, tx *sql.Tx) error {
	r, err := tx.ExecContext(ctx, `
		delete from assets
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

func (a *Asset) Renew(ctx context.Context, tx *sql.Tx, periods int64) error {
	r, err := tx.ExecContext(ctx, `
		update assets
		set expiry_time_unix = expiry_time_unix + (select duration_seconds * ? from asset_types where id = assets.asset_type_id)
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
