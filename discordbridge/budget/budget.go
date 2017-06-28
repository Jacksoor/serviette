package budget

import (
	"database/sql"
	"time"

	"golang.org/x/net/context"
)

type Budgeter struct {
	db *sql.DB

	maxBudget    time.Duration
	payoutPeriod time.Duration
}

func New(db *sql.DB, maxBudget time.Duration, payoutPeriod time.Duration) *Budgeter {
	return &Budgeter{
		db: db,

		maxBudget:    maxBudget,
		payoutPeriod: payoutPeriod,
	}
}

func (b *Budgeter) Remaining(ctx context.Context, userID string) (int64, error) {
	var remainingBudget int64

	if err := b.db.QueryRowContext(ctx, `
		insert into execution_budgets (user_id, remaining_budget, last_update_time)
		values ($1, $2, $3)
		on conflict (user_id) do update
		set remaining_budget = least(execution_budgets.remaining_budget + extract(epoch from ($3 - execution_budgets.last_update_time)) * $5 / $4, excluded.remaining_budget),
		    last_update_time = excluded.last_update_time
		returning remaining_budget
	`, userID, int64(b.maxBudget), time.Now(), int64(b.payoutPeriod), int64(time.Second)).Scan(&remainingBudget); err != nil {
		return 0, err
	}

	return remainingBudget, nil
}

func (b *Budgeter) Charge(ctx context.Context, userID string, cost time.Duration) error {
	if _, err := b.db.ExecContext(ctx, `
		update execution_budgets
		set remaining_budget = least(remaining_budget + extract(epoch from ($3 - last_update_time)) * $5 / $4 - $6, $2),
		    last_update_time = $3
		where user_id = $1
	`, userID, int64(b.maxBudget), time.Now(), int64(b.payoutPeriod), int64(time.Second), int64(cost)); err != nil {
		return err
	}
	return nil
}
