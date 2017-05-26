package accounts

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"golang.org/x/net/context"
	"strings"
	"time"

	deedspb "github.com/porpoises/kobun4/bank/deedsservice/v1pb"
)

var (
	ErrNotFound       error = errors.New("not found")
	ErrNoSuchDeedType       = errors.New("no such deed type")
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

func (s *Store) DeedType(ctx context.Context, tx *sql.Tx, name string) (*deedspb.TypeDefinition, error) {
	def := &deedspb.TypeDefinition{
		Name: name,
	}
	if err := tx.QueryRowContext(ctx, `
		select price, duration_seconds from deed_types
		where name = ?
	`, name).Scan(&def.Price, &def.DurationSeconds); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return def, nil
}

func (s *Store) DeedTypes(ctx context.Context, tx *sql.Tx) ([]*deedspb.TypeDefinition, error) {
	rows, err := tx.QueryContext(ctx, `
		select name, price, duration_seconds from deed_types
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	defs := make([]*deedspb.TypeDefinition, 0)

	for rows.Next() {
		def := &deedspb.TypeDefinition{}
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

func (s *Store) SetDeedTypes(ctx context.Context, tx *sql.Tx, defs []*deedspb.TypeDefinition) error {
	known := make([]interface{}, len(defs))
	for i, def := range defs {
		known[i] = def.Name
	}

	if len(known) > 0 {
		placeholders := strings.Repeat("?,", len(known))
		placeholders = placeholders[:len(placeholders)-1]

		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
			delete from deed_types
			where name not in (%s)
		`, placeholders), known...); err != nil {
			return err
		}

		for _, def := range defs {
			if _, err := tx.ExecContext(ctx, `
				update deed_types
				set price = ?, duration_seconds = ?
				where name = ?
			`, def.Price, def.DurationSeconds, def.Name); err != nil {
				return err
			}

			if _, err := tx.ExecContext(ctx, `
				insert or ignore into deed_types (name, price, duration_seconds)
				values (?, ?, ?)
			`, def.Name, def.Price, def.DurationSeconds); err != nil {
				return err
			}
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			delete from deed_types
		`); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) Deed(ctx context.Context, tx *sql.Tx, typ string, name string) (*Deed, error) {
	if err := expireDeeds(ctx, tx); err != nil {
		return nil, err
	}

	var id int64

	if err := tx.QueryRowContext(ctx, `
		select deeds.id from deeds
		inner join deed_types on deeds.deed_type_id = deed_types.id
		where deed_types.name = ? and deeds.name = ?
	`, typ, name).Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &Deed{
		id: id,
	}, nil
}

func (s *Store) Deeds(ctx context.Context, tx *sql.Tx) ([]*Deed, error) {
	if err := expireDeeds(ctx, tx); err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(ctx, `
		select id from deeds
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	deeds := make([]*Deed, 0)

	for rows.Next() {
		deed := &Deed{}
		if err := rows.Scan(&deed.id); err != nil {
			return nil, err
		}
		deeds = append(deeds, deed)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return deeds, nil
}

func (s *Store) DeedsByType(ctx context.Context, tx *sql.Tx, typ string) ([]*Deed, error) {
	if err := expireDeeds(ctx, tx); err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(ctx, `
		select deeds.id from deeds
		inner join deed_types on deeds.deed_type_id = deed_types.id
		deed_types.name = ?
	`, typ)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	deeds := make([]*Deed, 0)

	for rows.Next() {
		deed := &Deed{}
		if err := rows.Scan(&deed.id); err != nil {
			return nil, err
		}
		deeds = append(deeds, deed)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return deeds, nil
}

func (s *Store) AddDeed(ctx context.Context, tx *sql.Tx, typ string, name string, ownerAccountHandle []byte, periods int64, content []byte) (*Deed, error) {
	if err := expireDeeds(ctx, tx); err != nil {
		return nil, err
	}

	now := time.Now()

	var typeID int64
	var durationSeconds int64

	if err := tx.QueryRowContext(ctx, `
		select id, duration_seconds from deed_types
		where name = ?
	`, typ).Scan(&typeID, &durationSeconds); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNoSuchDeedType
		}
		return nil, err
	}

	r, err := tx.ExecContext(ctx, `
		insert into deeds (deed_type_id, name, owner_account_handle, expiry_time_unix, content)
		values (?, ?, ?, ?, ?)
	`, typeID, name, ownerAccountHandle, now.Add(time.Duration(durationSeconds)*time.Second*time.Duration(periods)).Unix(), content)
	if err != nil {
		return nil, err
	}

	id, err := r.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &Deed{
		id: id,
	}, nil
}

func expireDeeds(ctx context.Context, tx *sql.Tx) error {
	now := time.Now()

	_, err := tx.ExecContext(ctx, `
		delete from deeds
		where expiry_time_unix <= ?
	`, now.Unix())

	return err
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
