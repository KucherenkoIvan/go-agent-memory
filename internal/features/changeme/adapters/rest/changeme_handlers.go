// Package rest is the HTTP transport adapter. The containment rule applies:
// gin.Context never leaves this package — handlers bind, call use-cases with
// context.Context and typed arguments, respond. Errors go through
// httpapi.Fail; never hand-written error JSON.
package rest

import (
	"net/http"

	"github.com/KucherenkoIvan/go-kernel/httpapi"
	"github.com/gin-gonic/gin"

	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/application/usecases/managechangeme"
	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/domain"
)

type Handlers struct {
	create *managechangeme.CreateCommand
	update *managechangeme.UpdateCommand
	delete *managechangeme.DeleteCommand
	get    *managechangeme.GetQuery
	list   *managechangeme.ListQuery
}

func NewHandlers(
	create *managechangeme.CreateCommand,
	update *managechangeme.UpdateCommand,
	del *managechangeme.DeleteCommand,
	get *managechangeme.GetQuery,
	list *managechangeme.ListQuery,
) *Handlers {
	return &Handlers{create: create, update: update, delete: del, get: get, list: list}
}

func (h *Handlers) RegisterRoutes(r gin.IRouter) {
	r.POST("/changeme", h.Create)
	r.GET("/changeme", h.List)
	r.GET("/changeme/:id", h.Get)
	r.PUT("/changeme/:id", h.Update)
	r.DELETE("/changeme/:id", h.Delete)
}

type upsertRequest struct {
	Name string `json:"name" binding:"required"`
}

func (h *Handlers) Create(c *gin.Context) {
	var req upsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.Fail(c, httpapi.BadRequest("invalid_request", err))
		return
	}

	id, err := h.create.Execute(c.Request.Context(), req.Name)
	if err != nil {
		httpapi.Fail(c, err) // domain errors map via the error middleware
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *Handlers) Update(c *gin.Context) {
	var req upsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.Fail(c, httpapi.BadRequest("invalid_request", err))
		return
	}

	if err := h.update.Execute(c.Request.Context(), domain.ChangeMeID(c.Param("id")), req.Name); err != nil {
		httpapi.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handlers) Delete(c *gin.Context) {
	if err := h.delete.Execute(c.Request.Context(), domain.ChangeMeID(c.Param("id"))); err != nil {
		httpapi.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handlers) Get(c *gin.Context) {
	model, err := h.get.Execute(c.Request.Context(), domain.ChangeMeID(c.Param("id")))
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, model)
}

func (h *Handlers) List(c *gin.Context) {
	models, err := h.list.Execute(c.Request.Context())
	if err != nil {
		httpapi.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": models})
}
