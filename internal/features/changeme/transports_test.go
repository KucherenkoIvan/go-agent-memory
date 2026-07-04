package changeme_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	changemev1 "github.com/KucherenkoIvan/go-kernel/contracts/gen/grpc/changeme/v1"
	"github.com/KucherenkoIvan/go-kernel/events"
	"github.com/KucherenkoIvan/go-kernel/grpckit"
	"github.com/KucherenkoIvan/go-kernel/grpckit/grpckittest"
	"github.com/KucherenkoIvan/go-kernel/httpapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme"
	"github.com/KucherenkoIvan/go-tinycore-template/internal/shared/infra/storage"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func setupFeatureWithPub(t *testing.T) (*changeme.Feature, *events.ChannelPublisher) {
	t.Helper()
	db, err := storage.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	pub := events.NewChannelPublisher(events.WithLogger(discardLogger()))
	t.Cleanup(func() { _ = pub.Close(context.Background()) })

	return changeme.New(db, pub), pub
}

func setupFeature(t *testing.T) *changeme.Feature {
	t.Helper()
	feature, _ := setupFeatureWithPub(t)
	return feature
}

// newRESTHelper mounts the feature exactly like main.go does and returns a
// request function.
func newRESTHelper(t *testing.T, feature *changeme.Feature) func(method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httpapi.NewRouter(
		httpapi.WithLogger(discardLogger()),
		httpapi.WithErrorStatus("changeme_not_found", http.StatusNotFound),
	)
	feature.Handlers.RegisterRoutes(r.Group("/api"))

	return func(method, path, body string) *httptest.ResponseRecorder {
		var reader io.Reader
		if body != "" {
			reader = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, path, reader)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
}

// REST over httptest — the full CRUD surface with the uniform error contract.
func TestRESTAPI(t *testing.T) {
	do := newRESTHelper(t, setupFeature(t))

	// create
	w := do("POST", "/api/changeme", `{"name": "first"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("create response: %s", w.Body.String())
	}

	// binding error vs domain error — both uniform, different codes
	if w = do("POST", "/api/changeme", `{}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_request") {
		t.Fatalf("missing name: %d %s", w.Code, w.Body.String())
	}
	if w = do("POST", "/api/changeme", `{"name": "  "}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_name") {
		t.Fatalf("blank name: %d %s", w.Code, w.Body.String())
	}

	// get single by id
	w = do("GET", "/api/changeme/"+created.ID, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"name":"first"`) {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}
	if w = do("GET", "/api/changeme/missing", ""); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "changeme_not_found") {
		t.Fatalf("get missing: %d %s", w.Code, w.Body.String())
	}

	// update
	if w = do("PUT", "/api/changeme/"+created.ID, `{"name": "second"}`); w.Code != http.StatusNoContent {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}

	// list
	w = do("GET", "/api/changeme", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"name":"second"`) {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}

	// delete
	if w = do("DELETE", "/api/changeme/"+created.ID, ""); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	if w = do("DELETE", "/api/changeme/"+created.ID, ""); w.Code != http.StatusNotFound {
		t.Fatalf("double delete: %d %s", w.Code, w.Body.String())
	}
	if w = do("GET", "/api/changeme", ""); !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("list after delete: %s", w.Body.String())
	}
}

// gRPC over an in-memory connection — same use-cases, richer error transport.
func TestGRPCAPI(t *testing.T) {
	feature := setupFeature(t)
	srv := grpckit.NewServer(
		grpckit.WithLogger(discardLogger()),
		grpckit.WithErrorCode("changeme_not_found", codes.NotFound),
	)
	changemev1.RegisterChangeMeServiceServer(srv, feature.GRPC)
	client := changemev1.NewChangeMeServiceClient(grpckittest.Serve(t, srv))
	ctx := context.Background()

	// create
	created, err := client.Create(ctx, &changemev1.CreateRequest{Name: "first"})
	if err != nil || created.GetId() == "" {
		t.Fatalf("create: %v %v", created, err)
	}

	// domain error crosses the wire with its code and mapped status
	_, err = client.Get(ctx, &changemev1.GetRequest{Id: "missing"})
	var remote *grpckit.RemoteDomainError
	if !errors.As(err, &remote) || remote.Code != "changeme_not_found" || status.Code(err) != codes.NotFound {
		t.Fatalf("get missing: %v", err)
	}

	// get single by id
	got, err := client.Get(ctx, &changemev1.GetRequest{Id: created.GetId()})
	if err != nil || got.GetItem().GetName() != "first" || !got.GetItem().GetCreatedAt().IsValid() {
		t.Fatalf("get: %v %v", got, err)
	}

	// update + list
	if _, err := client.Update(ctx, &changemev1.UpdateRequest{Id: created.GetId(), Name: "second"}); err != nil {
		t.Fatal(err)
	}
	listed, err := client.List(ctx, &changemev1.ListRequest{})
	if err != nil || len(listed.GetItems()) != 1 || listed.GetItems()[0].GetName() != "second" {
		t.Fatalf("list: %v %v", listed, err)
	}

	// delete
	if _, err := client.Delete(ctx, &changemev1.DeleteRequest{Id: created.GetId()}); err != nil {
		t.Fatal(err)
	}
	listed, _ = client.List(ctx, &changemev1.ListRequest{})
	if len(listed.GetItems()) != 0 {
		t.Fatalf("list after delete: %v", listed)
	}
}
