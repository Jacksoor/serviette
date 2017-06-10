package varstore

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

func (s *Store) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return s.db.BeginTx(ctx, nil)
}

type GuildVars struct {
	ScriptCommandPrefix string
	MetaCommandPrefix   string
	Quiet               bool
	AdminRoleID         string
}

func (s *Store) GuildVars(ctx context.Context, tx *sql.Tx, guildID string) (*GuildVars, error) {
	guildVars := &GuildVars{}

	if err := tx.QueryRowContext(ctx, `
		select script_command_prefix, meta_command_prefix, quiet, admin_role_id
		from guild_vars
		where guild_id = ?
	`, guildID).Scan(&guildVars.ScriptCommandPrefix, &guildVars.MetaCommandPrefix, &guildVars.Quiet, &guildVars.AdminRoleID); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return guildVars, nil
}

func (s *Store) SetGuildVars(ctx context.Context, tx *sql.Tx, guildID string, guildVars *GuildVars) error {
	var r sql.Result
	var err error

	if guildVars == nil {
		r, err = tx.ExecContext(ctx, `
			delete from guild_vars
			where guild_id = ?
		`, guildID)
	} else {
		r, err = tx.ExecContext(ctx, `
			insert or replace into guild_vars (guild_id, script_command_prefix, meta_command_prefix, quiet, admin_role_id)
			values (?, ?, ?, ?, ?)
		`, guildID, guildVars.ScriptCommandPrefix, guildVars.MetaCommandPrefix, guildVars.Quiet, guildVars.AdminRoleID)
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

type Alias struct {
	OwnerName  string
	ScriptName string
}

func (s *Store) GuildAliases(ctx context.Context, tx *sql.Tx, guildID string) (map[string]*Alias, error) {
	aliases := make(map[string]*Alias)

	rows, err := tx.QueryContext(ctx, `
		select alias_name, owner_name, script_name
		from guild_aliases
		where guild_id = ?
	`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var aliasName string
		alias := &Alias{}

		if err := rows.Scan(&aliasName, &alias.OwnerName, &alias.ScriptName); err != nil {
			return nil, err
		}

		aliases[aliasName] = alias
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return aliases, nil
}

func (s *Store) GuildAlias(ctx context.Context, tx *sql.Tx, guildID string, aliasName string) (*Alias, error) {
	alias := &Alias{}

	if err := tx.QueryRowContext(ctx, `
		select owner_name, script_name
		from guild_aliases
		where guild_id = ? and
		      alias_name = ?
	`, guildID, aliasName).Scan(&alias.OwnerName, &alias.ScriptName); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return alias, nil
}

func (s *Store) SetGuildAlias(ctx context.Context, tx *sql.Tx, guildID string, aliasName string, alias *Alias) error {
	var r sql.Result
	var err error

	if alias == nil {
		r, err = tx.ExecContext(ctx, `
			delete from guild_aliases
			where guild_id = ? and
			      alias_name = ?
		`, guildID, aliasName)
	} else {
		r, err = tx.ExecContext(ctx, `
			insert or replace into guild_aliases (guild_id, alias_name, owner_name, script_name)
			values (?, ?, ?, ?)
		`, guildID, aliasName, alias.OwnerName, alias.ScriptName)
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
