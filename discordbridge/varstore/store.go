package varstore

import (
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
	"golang.org/x/net/context"
)

var (
	ErrNotFound     error = errors.New("not found")
	ErrInvalid            = errors.New("invalid")
	ErrNotPermitted       = errors.New("not permitted")
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
	Quiet                             bool
	Announcement                      string
	DeleteErrorsAfter                 time.Duration
	AllowUnprivilegedUnlinkedCommands bool
}

func (s *Store) CreateGuildVars(ctx context.Context, tx *sql.Tx, guildID string) error {
	if _, err := tx.ExecContext(ctx, `
		insert into guild_vars (guild_id)
		values ($1)
	`, guildID); err != nil {
		return err
	}

	return nil
}

func (s *Store) GuildVars(ctx context.Context, tx *sql.Tx, guildID string) (*GuildVars, error) {
	var deleteErrorsAfterSeconds int64

	guildVars := &GuildVars{}

	if err := tx.QueryRowContext(ctx, `
		select quiet, announcement, delete_errors_after_seconds, allow_unprivileged_unlinked_commands
		from guild_vars
		where guild_id = $1
	`, guildID).Scan(&guildVars.Quiet, &guildVars.Announcement, &deleteErrorsAfterSeconds, &guildVars.AllowUnprivilegedUnlinkedCommands); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	guildVars.DeleteErrorsAfter = time.Duration(deleteErrorsAfterSeconds) * time.Second

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
			insert into guild_vars (guild_id, quiet, announcement, delete_errors_after_seconds, allow_unprivileged_unlinked_commands)
			values ($1, $2, $3, $4, $5, $6)
			on conflict (guild_id) do update
			set quiet = excluded.quiet,
			    announcement = excluded.announcement,
			    delete_errors_after_seconds = excluded.delete_errors_after_seconds,
			    allow_unprivileged_unlinked_commands = excluded.allow_unprivileged_unlinked_commands
		`, guildID, guildVars.Quiet, guildVars.Announcement, int64(guildVars.DeleteErrorsAfter/time.Second), guildVars.AllowUnprivilegedUnlinkedCommands)
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

func (s *Store) FindLink(ctx context.Context, tx *sql.Tx, guildID string, content string) (string, *Link, error) {
	var linkName string
	link := &Link{}

	if err := tx.QueryRowContext(ctx, `
		select link_name, owner_name, script_name
		from guild_links
		where guild_id = $1 and
		      $2 ~* ('^' || regexp_replace(link_name, '\W', '\\\&', 'g') || '(?!\w)')
		order by length(link_name) desc
		limit 1
	`, guildID, content).Scan(&linkName, &link.OwnerName, &link.ScriptName); err != nil {
		if err == sql.ErrNoRows {
			return "", nil, ErrNotFound
		}
		return "", nil, err
	}

	return linkName, link, nil
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
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "22001" /* string_data_right_truncation */ {
			return ErrInvalid
		}
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

func (s *Store) Refcount(ctx context.Context, tx *sql.Tx, guildID string, ownerName string, scriptName string) (int, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `
		select count(1) from guild_links
		where guild_id = $1 and
		      owner_name = $2 and
		      script_name = $3
	`, guildID, ownerName, scriptName).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
