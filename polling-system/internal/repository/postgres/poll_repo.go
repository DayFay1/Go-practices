package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"polling-system/internal/domain/poll"
)

type PollRepo struct {
	db *sql.DB
}

func NewPollRepo(db *sql.DB) *PollRepo {
	return &PollRepo{db: db}
}

func (r *PollRepo) Create(ctx context.Context, p *poll.Poll, options []poll.Option) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	queryPoll := `
        INSERT INTO polls (title, description, status, starts_at, ends_at, creator_id)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id, created_at, updated_at
    `

	err = tx.QueryRowContext(ctx, queryPoll,
		p.Title,
		p.Description,
		p.Status,
		p.StartsAt,
		p.EndsAt,
		p.CreatorID,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return 0, err
	}

	queryOpt := `
        INSERT INTO options (poll_id, text)
        VALUES ($1, $2)
        RETURNING id, created_at
    `

	for i := range options {
		options[i].PollID = p.ID
		if err := tx.QueryRowContext(ctx, queryOpt, options[i].PollID, options[i].Text).
			Scan(&options[i].ID, &options[i].CreatedAt); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return p.ID, nil
}

func (r *PollRepo) GetByID(ctx context.Context, id int64) (*poll.Poll, []poll.Option, error) {
	p := &poll.Poll{}
	err := r.db.QueryRowContext(ctx, `
        SELECT id, title, description, status, starts_at, ends_at, creator_id, created_at, updated_at
        FROM polls WHERE id = $1
    `, id).Scan(
		&p.ID, &p.Title, &p.Description, &p.Status,
		&p.StartsAt, &p.EndsAt, &p.CreatorID, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, nil, err
	}

	rows, err := r.db.QueryContext(ctx, `
        SELECT id, poll_id, text, created_at
        FROM options WHERE poll_id = $1
    `, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var opts []poll.Option
	for rows.Next() {
		var o poll.Option
		if err := rows.Scan(&o.ID, &o.PollID, &o.Text, &o.CreatedAt); err != nil {
			return nil, nil, err
		}
		opts = append(opts, o)
	}

	return p, opts, nil
}

func (r *PollRepo) List(ctx context.Context, status *string) ([]poll.Poll, error) {
	query := `
        SELECT id, title, description, status, starts_at, ends_at, creator_id, created_at, updated_at
        FROM polls
    `
	var rows *sql.Rows
	var err error

	if status != nil {
		query += " WHERE status = $1 ORDER BY created_at DESC"
		rows, err = r.db.QueryContext(ctx, query, *status)
	} else {
		query += " ORDER BY created_at DESC"
		rows, err = r.db.QueryContext(ctx, query)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []poll.Poll
	for rows.Next() {
		var p poll.Poll
		if err := rows.Scan(&p.ID, &p.Title, &p.Description, &p.Status,
			&p.StartsAt, &p.EndsAt, &p.CreatorID, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		res = append(res, p)
	}
	return res, nil
}

func (r *PollRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE polls SET status = $1, updated_at = now() WHERE id = $2`, status, id)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *PollRepo) Update(ctx context.Context, id int64, input poll.UpdateInput) error {
	setParts := make([]string, 0, 4)
	args := make([]any, 0, 5)
	idx := 1

	if input.Title != nil {
		setParts = append(setParts, fmt.Sprintf("title = $%d", idx))
		args = append(args, *input.Title)
		idx++
	}
	if input.Description.Set {
		if input.Description.Value == nil {
			setParts = append(setParts, "description = NULL")
		} else {
			setParts = append(setParts, fmt.Sprintf("description = $%d", idx))
			args = append(args, *input.Description.Value)
			idx++
		}
	}
	if input.StartsAt.Set {
		if input.StartsAt.Value == nil {
			setParts = append(setParts, "starts_at = NULL")
		} else {
			setParts = append(setParts, fmt.Sprintf("starts_at = $%d", idx))
			args = append(args, *input.StartsAt.Value)
			idx++
		}
	}
	if input.EndsAt.Set {
		if input.EndsAt.Value == nil {
			setParts = append(setParts, "ends_at = NULL")
		} else {
			setParts = append(setParts, fmt.Sprintf("ends_at = $%d", idx))
			args = append(args, *input.EndsAt.Value)
			idx++
		}
	}

	if len(setParts) == 0 {
		return fmt.Errorf("no fields to update")
	}

	setParts = append(setParts, "updated_at = now()")
	query := fmt.Sprintf("UPDATE polls SET %s WHERE id = $%d", strings.Join(setParts, ", "), idx)
	args = append(args, id)

	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *PollRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM polls WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
