package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/KucherenkoIvan/go-kernel/ddd"
	kernelsqlite "github.com/KucherenkoIvan/go-kernel/sqlite"

	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/domain"
)

type ChangeMeReader struct {
	db *kernelsqlite.Client
}

func NewChangeMeReader(db *kernelsqlite.Client) *ChangeMeReader {
	return &ChangeMeReader{db: db}
}

func (r *ChangeMeReader) GetByID(ctx context.Context, tx ddd.Transaction, id domain.ChangeMeID) (*domain.ChangeMeReadModel, error) {
	row := r.db.Resolve(tx).QueryRowContext(ctx,
		`SELECT id, name, created_at FROM changeme WHERE id = $1`, string(id))

	model, err := scanReadModel(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading changeme: %w", err)
	}
	return &model, nil
}

func (r *ChangeMeReader) List(ctx context.Context, tx ddd.Transaction) ([]domain.ChangeMeReadModel, error) {
	rows, err := r.db.Resolve(tx).QueryContext(ctx,
		`SELECT id, name, created_at FROM changeme ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing changeme: %w", err)
	}
	defer rows.Close() //nolint:errcheck // read-only cursor

	models := []domain.ChangeMeReadModel{}
	for rows.Next() {
		model, err := scanReadModel(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("listing changeme: %w", err)
		}
		models = append(models, model)
	}
	return models, rows.Err()
}

func scanReadModel(scan func(dest ...any) error) (domain.ChangeMeReadModel, error) {
	var (
		model     domain.ChangeMeReadModel
		createdAt string
	)
	if err := scan(&model.ID, &model.Name, &createdAt); err != nil {
		return model, err
	}
	var err error
	if model.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return model, fmt.Errorf("parsing created_at: %w", err)
	}
	return model, nil
}
