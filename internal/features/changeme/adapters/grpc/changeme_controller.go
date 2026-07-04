// Package grpc is the gRPC transport adapter: thin, maps proto types to
// use-case calls, returns domain errors as-is — grpckit's interceptor chain
// encodes them into rich statuses.
package grpc

import (
	"context"

	changemev1 "github.com/KucherenkoIvan/go-kernel/contracts/gen/grpc/changeme/v1"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/application/usecases/managechangeme"
	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/domain"
)

type ChangeMeController struct {
	changemev1.UnimplementedChangeMeServiceServer

	create *managechangeme.CreateCommand
	update *managechangeme.UpdateCommand
	delete *managechangeme.DeleteCommand
	get    *managechangeme.GetQuery
	list   *managechangeme.ListQuery
}

func NewChangeMeController(
	create *managechangeme.CreateCommand,
	update *managechangeme.UpdateCommand,
	del *managechangeme.DeleteCommand,
	get *managechangeme.GetQuery,
	list *managechangeme.ListQuery,
) *ChangeMeController {
	return &ChangeMeController{create: create, update: update, delete: del, get: get, list: list}
}

func (c *ChangeMeController) Create(ctx context.Context, req *changemev1.CreateRequest) (*changemev1.CreateResponse, error) {
	id, err := c.create.Execute(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	return &changemev1.CreateResponse{Id: string(id)}, nil
}

func (c *ChangeMeController) Update(ctx context.Context, req *changemev1.UpdateRequest) (*changemev1.UpdateResponse, error) {
	if err := c.update.Execute(ctx, domain.ChangeMeID(req.GetId()), req.GetName()); err != nil {
		return nil, err
	}
	return &changemev1.UpdateResponse{}, nil
}

func (c *ChangeMeController) Delete(ctx context.Context, req *changemev1.DeleteRequest) (*changemev1.DeleteResponse, error) {
	if err := c.delete.Execute(ctx, domain.ChangeMeID(req.GetId())); err != nil {
		return nil, err
	}
	return &changemev1.DeleteResponse{}, nil
}

func (c *ChangeMeController) Get(ctx context.Context, req *changemev1.GetRequest) (*changemev1.GetResponse, error) {
	model, err := c.get.Execute(ctx, domain.ChangeMeID(req.GetId()))
	if err != nil {
		return nil, err
	}
	return &changemev1.GetResponse{Item: toProto(*model)}, nil
}

func (c *ChangeMeController) List(ctx context.Context, _ *changemev1.ListRequest) (*changemev1.ListResponse, error) {
	models, err := c.list.Execute(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]*changemev1.ChangeMe, 0, len(models))
	for _, model := range models {
		items = append(items, toProto(model))
	}
	return &changemev1.ListResponse{Items: items}, nil
}

func toProto(model domain.ChangeMeReadModel) *changemev1.ChangeMe {
	return &changemev1.ChangeMe{
		Id:        model.ID,
		Name:      model.Name,
		CreatedAt: timestamppb.New(model.CreatedAt),
	}
}
