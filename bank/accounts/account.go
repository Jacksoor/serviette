package accounts

import (
	"database/sql"
	"encoding/binary"
	"golang.org/x/net/context"
	"time"
)

type Account struct {
	id  int64
	key []byte
}

func (a *Account) Handle() []byte {
	handle := make([]byte, 64/8)
	binary.BigEndian.PutUint64(handle, uint64(a.id))
	return handle
}

func (a *Account) Key() []byte {
	return a.key
}

func (a *Account) Balance(ctx context.Context, tx *sql.Tx) (int64, error) {
	var balance int64

	if err := tx.QueryRowContext(ctx, `
		select balance from accounts
		where id = ?
	`, a.id).Scan(&balance); err != nil {
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
		where id = ?
	`, amount, a.id)
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

func (a *Account) Asset(ctx context.Context, tx *sql.Tx, typ string, name string) (*Asset, error) {
	if err := expireAssets(ctx, tx); err != nil {
		return nil, err
	}

	var id int64

	if err := tx.QueryRowContext(ctx, `
		select id from assets
		where account_id = ? and type = ? and name = ?
	`, a.id, typ, name).Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &Asset{
		id: id,
	}, nil
}

type AssetEntry struct {
	Type       string
	Name       string
	ExpiryTime time.Time
}

func (a *Account) Assets(ctx context.Context, tx *sql.Tx) ([]*Asset, error) {
	if err := expireAssets(ctx, tx); err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(ctx, `
		select id from assets
		where account_id = ?
	`, a.id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	assets := make([]*Asset, 0)

	for rows.Next() {
		asset := &Asset{}
		if err := rows.Scan(&asset.id); err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return assets, nil
}

func (a *Account) AddAsset(ctx context.Context, tx *sql.Tx, typ string, name string, periods int64, content []byte) (*Asset, error) {
	if err := expireAssets(ctx, tx); err != nil {
		return nil, err
	}

	now := time.Now()

	var typeID int64
	var durationSeconds int64

	if err := tx.QueryRowContext(ctx, `
		select id, duration_seconds from asset_types
		where name = ?
	`, typ).Scan(&typeID, &durationSeconds); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNoSuchAssetType
		}
		return nil, err
	}

	r, err := tx.ExecContext(ctx, `
		insert into assets (account_id, asset_type_id, name, content, expiry_time_unix)
		values (?, ?, ?, ?, ?)
	`, a.id, typeID, name, content, now.Add(time.Duration(durationSeconds)*time.Second*time.Duration(periods)).Unix())
	if err != nil {
		return nil, err
	}

	id, err := r.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &Asset{
		id: id,
	}, nil
}

func (a *Account) Delete(ctx context.Context, tx *sql.Tx) error {
	r, err := tx.ExecContext(ctx, `
		delete from accounts
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
