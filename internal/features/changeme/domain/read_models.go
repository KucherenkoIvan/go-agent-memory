package domain

import "time"

// ChangeMeReadModel is the query view — read-only by convention, shaped for
// transports (json tags for REST; the gRPC adapter maps it to proto).
type ChangeMeReadModel struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}
