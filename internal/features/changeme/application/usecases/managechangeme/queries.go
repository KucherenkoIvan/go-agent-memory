package managechangeme

import (
	"context"

	"github.com/KucherenkoIvan/go-kernel/ddd"

	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/application/ports"
	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/domain"
)

// GetQuery — single by id, through the reader. Missing is part of the
// query's contract (a domain error), so transports map it uniformly.
type GetQuery struct {
	reader ports.ChangeMeReader
}

func NewGetQuery(reader ports.ChangeMeReader) *GetQuery {
	return &GetQuery{reader: reader}
}

func (q *GetQuery) Execute(ctx context.Context, id domain.ChangeMeID) (*domain.ChangeMeReadModel, error) {
	model, err := q.reader.GetByID(ctx, ddd.NoTransaction, id)
	if err != nil {
		return nil, err
	}
	if model == nil {
		return nil, &domain.ChangeMeNotFoundError{}
	}
	return model, nil
}

// ListQuery — all of them. Real projects add pagination here before the
// table grows teeth.
type ListQuery struct {
	reader ports.ChangeMeReader
}

func NewListQuery(reader ports.ChangeMeReader) *ListQuery {
	return &ListQuery{reader: reader}
}

func (q *ListQuery) Execute(ctx context.Context) ([]domain.ChangeMeReadModel, error) {
	return q.reader.List(ctx, ddd.NoTransaction)
}
