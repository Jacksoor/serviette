package varstore

import (
	"database/sql"
	"errors"
	"time"

	"golang.org/x/net/context"
)

var (
	ErrNotFound error = errors.New("not found")
)

type UserVars struct {
	AccountHandle  []byte
	LastPayoutTime time.Time
}

type ChannelVars struct {
	MinPayout int64
	MaxPayout int64
	Cooldown  time.Duration
}

type GuildVars struct {
	ScriptCommandPrefix string
	BankCommandPrefix   string
	CurrencyName        string
	Quiet               bool
}

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

func (s *Store) UserVars(ctx context.Context, tx *sql.Tx, userID string) (*UserVars, error) {
	userVars := &UserVars{}

	var lastPayoutTimeUnix int64

	if err := tx.QueryRowContext(ctx, `
		select account_handle, last_payout_time_unix
		from user_vars
		where user_id = ?
	`, userID).Scan(&userVars.AccountHandle, &lastPayoutTimeUnix); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	userVars.LastPayoutTime = time.Unix(lastPayoutTimeUnix, 0)

	return userVars, nil
}

func (s *Store) SetUserVars(ctx context.Context, tx *sql.Tx, userID string, userVars *UserVars) error {
	var r sql.Result
	var err error

	if userVars == nil {
		r, err = tx.ExecContext(ctx, `
			delete from user_vars
			where user_id = ?
		`, userID)
	} else {
		r, err = tx.ExecContext(ctx, `
			update user_vars
			set account_handle = ?,
			    last_payout_time_unix = ?
			where user_id = ?
		`, userVars.AccountHandle, userVars.LastPayoutTime.Unix(), userID)
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

func (s *Store) ChannelVars(ctx context.Context, tx *sql.Tx, channelID string) (*ChannelVars, error) {
	channelVars := &ChannelVars{}

	var cooldownSeconds int64

	if err := tx.QueryRowContext(ctx, `
		select min_payout, max_payout, cooldown_seconds
		from channel_vars
		where channel_id = ?
	`, channelID).Scan(&channelVars.MinPayout, &channelVars.MaxPayout, &cooldownSeconds); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	channelVars.Cooldown = time.Duration(cooldownSeconds) * time.Second

	return channelVars, nil
}

func (s *Store) SetChannelVars(ctx context.Context, tx *sql.Tx, channelID string, channelVars *ChannelVars) error {
	var r sql.Result
	var err error

	if channelVars == nil {
		r, err = tx.ExecContext(ctx, `
			delete from channel_vars
			where channel_id = ?
		`, channelID)
	} else {
		r, err = tx.ExecContext(ctx, `
			update channel_vars
			set min_payout = ?,
			    max_payout = ?,
			    cooldown_seconds = ?,
			where channel_id = ?
		`, channelVars.MinPayout, channelVars.MaxPayout, channelVars.Cooldown/time.Second, channelID)
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

func (s *Store) GuildVars(ctx context.Context, tx *sql.Tx, guildID string) (*GuildVars, error) {
	guildVars := &GuildVars{}

	if err := tx.QueryRowContext(ctx, `
		select script_command_prefix, bank_command_prefix, currency_name, quiet
		from guild_vars
		where guild_id = ?
	`, guildID).Scan(&guildVars.ScriptCommandPrefix, &guildVars.BankCommandPrefix, &guildVars.CurrencyName, &guildVars.Quiet); err != nil {
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
			update guild_vars
			set script_command_prefix = ?,
			    bank_command_prefix = ?,
			    currency_name = ?,
			    quiet = ?
			where guild_id = ?
		`, guildVars.ScriptCommandPrefix, guildVars.BankCommandPrefix, guildVars.CurrencyName, guildVars.Quiet, guildID)
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
