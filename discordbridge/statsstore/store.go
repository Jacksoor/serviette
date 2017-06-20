package statsstore

import (
	"database/sql"
	"time"

	"golang.org/x/net/context"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{
		db: db,
	}
}

type UserChannelStats struct {
	NumCharactersSent int64
	NumMessagesSent   int64
	LastResetTime     time.Time
}

func (s *Store) RecordUserChannelMessage(ctx context.Context, userID string, channelID string, messageLength int64) error {
	now := time.Now()

	if _, err := s.db.ExecContext(ctx, `
		insert into user_channel_stats (user_id, channel_id, num_characters_sent, num_messages_sent, last_reset_time)
		values ($1, $2, $3, 1, $4)
		on conflict (user_id, channel_id) do update
		set num_characters_sent = user_channel_stats.num_characters_sent + excluded.num_characters_sent,
		    num_messages_sent = user_channel_stats.num_messages_sent + excluded.num_messages_sent
	`, userID, channelID, messageLength, now); err != nil {
		return err
	}

	return nil
}

func (s *Store) UserChannelStats(ctx context.Context, userID string, channelID string) (*UserChannelStats, error) {
	stats := &UserChannelStats{}

	if err := s.db.QueryRowContext(ctx, `
		select num_characters_sent, num_messages_sent, last_reset_time
		from user_channel_stats
		where user_id = $1 and
		      channel_id = $2
	`, userID, channelID).Scan(&stats.NumCharactersSent, &stats.NumMessagesSent, &stats.LastResetTime); err != nil {
		if err != sql.ErrNoRows {
			return nil, err
		}
	}

	return stats, nil
}
