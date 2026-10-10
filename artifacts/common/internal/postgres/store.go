package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"devops-practice/internal/config"
	"devops-practice/internal/request"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound      = errors.New("request not found")
	ErrAlreadyExists = errors.New("request already exists")
)

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string, options config.Postgres) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL connection configuration")
	}
	cfg.MaxConns = options.MaxConns
	cfg.MinConns = options.MinConns
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = fmt.Sprint(options.StatementTimeout.Milliseconds())
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = fmt.Sprint(options.LockTimeout.Milliseconds())
	cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = fmt.Sprint(options.IdleInTransactionTimeout.Milliseconds())
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) PoolStats() *pgxpool.Stat {
	return s.pool.Stat()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) Create(ctx context.Context, value request.Request) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO requests (id, title, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`, value.ID, value.Title, value.Status, value.CreatedAt, value.UpdatedAt)
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrAlreadyExists
	}
	return fmt.Errorf("create request: %w", err)
}

func (s *Store) Get(ctx context.Context, id uuid.UUID) (request.Request, error) {
	var value request.Request
	err := s.pool.QueryRow(ctx, `
		SELECT id, title, status, created_at, updated_at
		FROM requests
		WHERE id = $1
	`, id).Scan(&value.ID, &value.Title, &value.Status, &value.CreatedAt, &value.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return request.Request{}, ErrNotFound
	}
	if err != nil {
		return request.Request{}, fmt.Errorf("get request: %w", err)
	}
	return value, nil
}

func (s *Store) MarkProcessed(ctx context.Context, id uuid.UUID, at time.Time) (bool, error) {
	result, err := s.pool.Exec(ctx, `
		UPDATE requests
		SET status = $2, updated_at = $3
		WHERE id = $1 AND status <> $2
	`, id, request.StatusProcessed, at)
	if err != nil {
		return false, fmt.Errorf("mark request processed: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

func (s *Store) Close() {
	s.pool.Close()
}
