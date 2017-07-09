package budget

import (
	"database/sql"
	"errors"
	"time"

	"golang.org/x/net/context"
)

var ErrBudgetExceeded error = errors.New("budget exceeded")

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

func (b *Budgeter) Charge(ctx context.Context, userID string, cost time.Duration) (int64, error) {
	var remainingBudget int64

	if err := b.db.QueryRowContext(ctx, `
		update execution_budgets
		set remaining_budget = least(remaining_budget + extract(epoch from ($3 - last_update_time)) * $5 / $4 - $6, $2),
		    last_update_time = $3
		where user_id = $1
		returning remaining_budget
	`, userID, int64(b.maxBudget), time.Now(), int64(b.payoutPeriod), int64(time.Second), int64(cost)).Scan(&remainingBudget); err != nil {
		return 0, err
	}
	return remainingBudget, nil
}

func (b *Budgeter) CheckedCharge(ctx context.Context, userID string, cost time.Duration) (int64, error) {
	var remainingBudget int64

	if err := b.db.QueryRowContext(ctx, `
		update execution_budgets
		set remaining_budget = least(remaining_budget + extract(epoch from ($3 - last_update_time)) * $5 / $4 - $6, $2),
		    last_update_time = $3
		where user_id = $1 and
		      remaining_budget + extract(epoch from ($3 - last_update_time)) * $5 / $4 - $6 >= 0
		returning remaining_budget
	`, userID, int64(b.maxBudget), time.Now(), int64(b.payoutPeriod), int64(time.Second), int64(cost)).Scan(&remainingBudget); err != nil {
		if err == sql.ErrNoRows {
			return 0, ErrBudgetExceeded
		}
		return 0, err
	}
	return remainingBudget, nil
}
