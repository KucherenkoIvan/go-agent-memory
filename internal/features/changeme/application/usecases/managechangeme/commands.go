// Package managechangeme holds the placeholder CRUD use-cases. Real
// use-cases follow exactly these shapes; queries get added alongside a
// Reader port when read scenarios appear.
package managechangeme

import (
	"context"

	"github.com/KucherenkoIvan/go-kernel/ddd"

	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/application/ports"
	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/domain"
)

// CreateCommand: build the aggregate, persist and publish atomically.
type CreateCommand struct {
	txManager ddd.TxManager
	ids       ddd.IDGenerator
	clock     ddd.Clock
	repo      ports.ChangeMeRepository
	events    ports.ChangeMeEventProducer
}

func NewCreateCommand(
	txManager ddd.TxManager,
	ids ddd.IDGenerator,
	clock ddd.Clock,
	repo ports.ChangeMeRepository,
	events ports.ChangeMeEventProducer,
) *CreateCommand {
	return &CreateCommand{txManager: txManager, ids: ids, clock: clock, repo: repo, events: events}
}

func (c *CreateCommand) Execute(ctx context.Context, name string) (domain.ChangeMeID, error) {
	aggregate, err := domain.NewChangeMe(domain.ChangeMeID(c.ids.NewID()), name, c.clock.Now())
	if err != nil {
		return "", err
	}

	err = c.txManager.WithinTx(ctx, func(tx ddd.Transaction) error {
		if err := c.repo.Save(ctx, tx, aggregate); err != nil {
			return err
		}
		return c.events.Publish(ctx, tx, aggregate.PopEvents()...)
	})
	if err != nil {
		return "", err
	}
	return aggregate.ID(), nil
}

// UpdateCommand: load, mutate through the aggregate, save, publish.
// Invariants live in the aggregate method, not here.
type UpdateCommand struct {
	txManager ddd.TxManager
	repo      ports.ChangeMeRepository
	events    ports.ChangeMeEventProducer
}

func NewUpdateCommand(txManager ddd.TxManager, repo ports.ChangeMeRepository, events ports.ChangeMeEventProducer) *UpdateCommand {
	return &UpdateCommand{txManager: txManager, repo: repo, events: events}
}

func (c *UpdateCommand) Execute(ctx context.Context, id domain.ChangeMeID, name string) error {
	return c.txManager.WithinTx(ctx, func(tx ddd.Transaction) error {
		aggregate, err := c.repo.GetByID(ctx, tx, id)
		if err != nil {
			return err
		}
		if aggregate == nil {
			return &domain.ChangeMeNotFoundError{}
		}
		if err := aggregate.Update(name); err != nil {
			return err
		}
		if err := c.repo.Save(ctx, tx, aggregate); err != nil {
			return err
		}
		return c.events.Publish(ctx, tx, aggregate.PopEvents()...)
	})
}

// DeleteCommand: verify existence, delete, publish the deletion fact.
type DeleteCommand struct {
	txManager ddd.TxManager
	repo      ports.ChangeMeRepository
	events    ports.ChangeMeEventProducer
}

func NewDeleteCommand(txManager ddd.TxManager, repo ports.ChangeMeRepository, events ports.ChangeMeEventProducer) *DeleteCommand {
	return &DeleteCommand{txManager: txManager, repo: repo, events: events}
}

func (c *DeleteCommand) Execute(ctx context.Context, id domain.ChangeMeID) error {
	return c.txManager.WithinTx(ctx, func(tx ddd.Transaction) error {
		aggregate, err := c.repo.GetByID(ctx, tx, id)
		if err != nil {
			return err
		}
		if aggregate == nil {
			return &domain.ChangeMeNotFoundError{}
		}
		if err := c.repo.Delete(ctx, tx, id); err != nil {
			return err
		}
		return c.events.Publish(ctx, tx, domain.NewChangeMeDeletedEvent(domain.ChangeMeDeletedData{ID: id}))
	})
}
