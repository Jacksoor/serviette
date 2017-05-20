package accounts

import (
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"golang.org/x/net/context"
	"strings"
	"time"

	assetspb "github.com/porpoises/kobun4/bank/assetsservice/v1pb"
)

var (
	ErrNotFound        error = errors.New("not found")
	ErrNoSuchAssetType       = errors.New("no such asset type")
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
	id := int64(binary.BigEndian.Uint64(handle))

	var key []byte
	if err := tx.QueryRowContext(ctx, `
		select key from accounts
		where id = ?
	`, id).Scan(&key); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &Account{
		id:  id,
		key: key,
	}, nil
}

func (s *Store) Create(ctx context.Context, tx *sql.Tx) (*Account, error) {
	key := make([]byte, 64/8)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}

	r, err := tx.ExecContext(ctx, `
		insert into accounts (key)
		values (?)
	`, key)
	if err != nil {
		return nil, err
	}

	id, err := r.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &Account{
		id:  id,
		key: key,
	}, nil
}

func (s *Store) AssetType(ctx context.Context, tx *sql.Tx, name string) (*assetspb.TypeDefinition, error) {
	def := &assetspb.TypeDefinition{
		Name: name,
	}
	if err := tx.QueryRowContext(ctx, `
		select price, duration_seconds from asset_types
		where name = ?
	`, name).Scan(&def.Price, &def.DurationSeconds); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return def, nil
}

func (s *Store) AssetTypes(ctx context.Context, tx *sql.Tx) ([]*assetspb.TypeDefinition, error) {
	rows, err := tx.QueryContext(ctx, `
		select name, price, duration_seconds from asset_types
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	defs := make([]*assetspb.TypeDefinition, 0)

	for rows.Next() {
		def := &assetspb.TypeDefinition{}
		if err := rows.Scan(&def.Name, &def.Price, &def.DurationSeconds); err != nil {
			return nil, err
		}
		defs = append(defs, def)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return defs, nil
}

func (s *Store) SetAssetTypes(ctx context.Context, tx *sql.Tx, defs []*assetspb.TypeDefinition) error {
	known := make([]interface{}, len(defs))
	for i, def := range defs {
		known[i] = def.Name
	}

	if len(known) > 0 {
		placeholders := strings.Repeat("?,", len(known))
		placeholders = placeholders[:len(placeholders)-1]

		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
			delete from asset_types
			where name not in (%s)
		`, placeholders), known...); err != nil {
			return err
		}

		for _, def := range defs {
			if _, err := tx.ExecContext(ctx, `
				update asset_types
				set price = ?, duration_seconds = ?
				where name = ?
			`, def.Price, def.DurationSeconds, def.Name); err != nil {
				return err
			}

			if _, err := tx.ExecContext(ctx, `
				insert or ignore into asset_types (name, price, duration_seconds)
				values (?, ?, ?)
			`, def.Name, def.Price, def.DurationSeconds); err != nil {
				return err
			}
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			delete from asset_types
		`); err != nil {
			return err
		}
	}

	return nil
}

func expireAssets(ctx context.Context, tx *sql.Tx) error {
	now := time.Now()

	_, err := tx.ExecContext(ctx, `
		delete from assets
		where expiry_time_unix <= ?
	`, now.Unix())

	return err
}
