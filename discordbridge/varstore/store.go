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
	Quiet               bool
	AdminRoleID         string
	Announcement        string
}

func (s *Store) GuildVars(ctx context.Context, tx *sql.Tx, guildID string) (*GuildVars, error) {
	guildVars := &GuildVars{}

	if err := tx.QueryRowContext(ctx, `
		select script_command_prefix, quiet, admin_role_id, announcement
		from guild_vars
		where guild_id = $1
	`, guildID).Scan(&guildVars.ScriptCommandPrefix, &guildVars.Quiet, &guildVars.AdminRoleID, &guildVars.Announcement); err != nil {
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
			where guild_id = $1
		`, guildID)
	} else {
		r, err = tx.ExecContext(ctx, `
			insert into guild_vars (guild_id, script_command_prefix, quiet, admin_role_id, announcement)
			values ($1, $2, $3, $4, $5)
			on conflict (guild_id) do update
			set script_command_prefix = excluded.script_command_prefix,
			    quiet = excluded.quiet,
			    admin_role_id = excluded.admin_role_id,
			    announcement = excluded.announcement
		`, guildID, guildVars.ScriptCommandPrefix, guildVars.Quiet, guildVars.AdminRoleID, guildVars.Announcement)
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

type Link struct {
	OwnerName  string
	ScriptName string
}

func (s *Store) GuildLinks(ctx context.Context, tx *sql.Tx, guildID string) (map[string]*Link, error) {
	links := make(map[string]*Link)

	rows, err := tx.QueryContext(ctx, `
		select link_name, owner_name, script_name
		from guild_links
		where guild_id = $1
	`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var linkName string
		link := &Link{}

		if err := rows.Scan(&linkName, &link.OwnerName, &link.ScriptName); err != nil {
			return nil, err
		}

		links[linkName] = link
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return links, nil
}

func (s *Store) GuildLink(ctx context.Context, tx *sql.Tx, guildID string, linkName string) (*Link, error) {
	link := &Link{}

	if err := tx.QueryRowContext(ctx, `
		select owner_name, script_name
		from guild_links
		where guild_id = $1 and
		      link_name = $2
	`, guildID, linkName).Scan(&link.OwnerName, &link.ScriptName); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return link, nil
}

func (s *Store) SetGuildLink(ctx context.Context, tx *sql.Tx, guildID string, linkName string, link *Link) error {
	var r sql.Result
	var err error

	if link == nil {
		r, err = tx.ExecContext(ctx, `
			delete from guild_links
			where guild_id = $1 and
			      link_name = $2
		`, guildID, linkName)
	} else {
		r, err = tx.ExecContext(ctx, `
			insert into guild_links (guild_id, link_name, owner_name, script_name)
			values ($1, $2, $3, $4)
			on conflict (guild_id, link_name) do update
			set owner_name = excluded.owner_name,
			    script_name = excluded.script_name
		`, guildID, linkName, link.OwnerName, link.ScriptName)
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
