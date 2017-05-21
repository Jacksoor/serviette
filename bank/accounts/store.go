package accounts

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"golang.org/x/net/context"
	"strings"
	"time"

	namespb "github.com/porpoises/kobun4/bank/namesservice/v1pb"
)

var (
	ErrNotFound       error = errors.New("not found")
	ErrNoSuchNameType       = errors.New("no such name type")
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

func (s *Store) NameType(ctx context.Context, tx *sql.Tx, name string) (*namespb.TypeDefinition, error) {
	def := &namespb.TypeDefinition{
		Name: name,
	}
	if err := tx.QueryRowContext(ctx, `
		select price, duration_seconds from name_types
		where name = ?
	`, name).Scan(&def.Price, &def.DurationSeconds); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return def, nil
}

func (s *Store) NameTypes(ctx context.Context, tx *sql.Tx) ([]*namespb.TypeDefinition, error) {
	rows, err := tx.QueryContext(ctx, `
		select name, price, duration_seconds from name_types
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	defs := make([]*namespb.TypeDefinition, 0)

	for rows.Next() {
		def := &namespb.TypeDefinition{}
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

func (s *Store) SetNameTypes(ctx context.Context, tx *sql.Tx, defs []*namespb.TypeDefinition) error {
	known := make([]interface{}, len(defs))
	for i, def := range defs {
		known[i] = def.Name
	}

	if len(known) > 0 {
		placeholders := strings.Repeat("?,", len(known))
		placeholders = placeholders[:len(placeholders)-1]

		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
			delete from name_types
			where name not in (%s)
		`, placeholders), known...); err != nil {
			return err
		}

		for _, def := range defs {
			if _, err := tx.ExecContext(ctx, `
				update name_types
				set price = ?, duration_seconds = ?
				where name = ?
			`, def.Price, def.DurationSeconds, def.Name); err != nil {
				return err
			}

			if _, err := tx.ExecContext(ctx, `
				insert or ignore into name_types (name, price, duration_seconds)
				values (?, ?, ?)
			`, def.Name, def.Price, def.DurationSeconds); err != nil {
				return err
			}
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			delete from name_types
		`); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) Name(ctx context.Context, tx *sql.Tx, typ string, name string) (*Name, error) {
	if err := expireNames(ctx, tx); err != nil {
		return nil, err
	}

	var id int64

	if err := tx.QueryRowContext(ctx, `
        select names.id from names
        inner join name_types on names.name_type_id = name_types.id
        where name_types.name = ? and names.name = ?
    `, typ, name).Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &Name{
		id: id,
	}, nil
}

func (s *Store) Names(ctx context.Context, tx *sql.Tx) ([]*Name, error) {
	if err := expireNames(ctx, tx); err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(ctx, `
        select id from names
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	names := make([]*Name, 0)

	for rows.Next() {
		name := &Name{}
		if err := rows.Scan(&name.id); err != nil {
			return nil, err
		}
		names = append(names, name)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return names, nil
}

func (s *Store) NamesByType(ctx context.Context, tx *sql.Tx, typ string) ([]*Name, error) {
	if err := expireNames(ctx, tx); err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(ctx, `
        select names.id from names
        inner join name_types on names.name_type_id = name_types.id
        name_types.name = ?
    `, typ)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	names := make([]*Name, 0)

	for rows.Next() {
		name := &Name{}
		if err := rows.Scan(&name.id); err != nil {
			return nil, err
		}
		names = append(names, name)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return names, nil
}

func (s *Store) AddName(ctx context.Context, tx *sql.Tx, typ string, name string, ownerAccountHandle []byte, periods int64, content []byte) (*Name, error) {
	if err := expireNames(ctx, tx); err != nil {
		return nil, err
	}

	now := time.Now()

	var typeID int64
	var durationSeconds int64

	if err := tx.QueryRowContext(ctx, `
        select id, duration_seconds from name_types
        where name = ?
    `, typ).Scan(&typeID, &durationSeconds); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNoSuchNameType
		}
		return nil, err
	}

	r, err := tx.ExecContext(ctx, `
        insert into names (name_type_id, name, owner_account_handle, expiry_time_unix, content)
        values (?, ?, ?, ?, ?)
    `, typeID, name, ownerAccountHandle, now.Add(time.Duration(durationSeconds)*time.Second*time.Duration(periods)).Unix(), content)
	if err != nil {
		return nil, err
	}

	id, err := r.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &Name{
		id: id,
	}, nil
}

func expireNames(ctx context.Context, tx *sql.Tx) error {
	now := time.Now()

	_, err := tx.ExecContext(ctx, `
		delete from names
		where expiry_time_unix <= ?
	`, now.Unix())

	return err
}
