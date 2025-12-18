package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"polling-system/internal/domain/vote"
)

type VoteRepo struct {
	db *sql.DB
}

func NewVoteRepo(db *sql.DB) *VoteRepo {
	return &VoteRepo{db: db}
}

func (r *VoteRepo) Create(ctx context.Context, v *vote.Vote) error {
	query := `
        INSERT INTO votes (poll_id, option_id, user_id)
        VALUES ($1, $2, $3)
        RETURNING id, created_at
    `
	err := r.db.QueryRowContext(ctx, query, v.PollID, v.OptionID, v.UserID).
		Scan(&v.ID, &v.CreatedAt)
	if err != nil {
		return mapVoteError(err)
	}
	return nil
}

func (r *VoteRepo) CountByPoll(ctx context.Context, pollID int64) (map[int64]int64, int64, error) {
	rows, err := r.db.QueryContext(ctx, `
        SELECT option_id, COUNT(*)
        FROM votes
        WHERE poll_id = $1
        GROUP BY option_id
    `, pollID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	res := make(map[int64]int64)
	var total int64
	for rows.Next() {
		var optID int64
		var c int64
		if err := rows.Scan(&optID, &c); err != nil {
			return nil, 0, err
		}
		res[optID] = c
		total += c
	}

	return res, total, nil
}

func (r *VoteRepo) TotalVotes(ctx context.Context, pollID int64) (int64, error) {
	var total int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM votes WHERE poll_id = $1`, pollID).Scan(&total)
	return total, err
}

func (r *VoteRepo) AggregatedByPoll(ctx context.Context, pollID int64) (map[int64]int64, int64, error) {
	rows, err := r.db.QueryContext(ctx, `
        SELECT option_id, votes_count
        FROM aggregated_results
        WHERE poll_id = $1
    `, pollID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	res := make(map[int64]int64)
	var total int64
	for rows.Next() {
		var optID int64
		var c int64
		if err := rows.Scan(&optID, &c); err != nil {
			return nil, 0, err
		}
		res[optID] = c
		total += c
	}
	return res, total, nil
}

func (r *VoteRepo) IncrementAggregated(ctx context.Context, pollID, optionID int64) error {
	_, err := r.db.ExecContext(ctx, `
        INSERT INTO aggregated_results (poll_id, option_id, votes_count)
        VALUES ($1, $2, 1)
        ON CONFLICT (poll_id, option_id) DO UPDATE
        SET votes_count = aggregated_results.votes_count + 1,
            updated_at = now()
    `, pollID, optionID)
	return err
}

func (r *VoteRepo) GetPollStatus(ctx context.Context, pollID int64) (string, error) {
	var status string
	err := r.db.QueryRowContext(ctx, `SELECT status FROM polls WHERE id = $1`, pollID).Scan(&status)
	return status, err
}

func mapVoteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			if pgErr.ConstraintName == "votes_poll_id_user_id_key" || pgErr.ConstraintName == "votes_poll_id_user_id_idx" {
				return vote.ErrAlreadyVoted
			}
			return vote.ErrAlreadyVoted
		case "23503":
			switch pgErr.ConstraintName {
			case "votes_option_poll_fkey", "votes_option_id_fkey":
				return vote.ErrOptionNotInPoll
			case "votes_poll_id_fkey":
				return vote.ErrPollNotFound
			}
		}
	}
	return err
}
