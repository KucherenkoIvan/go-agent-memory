package domain

import "github.com/KucherenkoIvan/go-kernel/ddd"

// CRUD events — rename together with the aggregate. Real domains usually
// deserve better names than "updated" (state what happened in business
// terms), but the shape stays exactly this.
const (
	ChangeMeCreatedEventName = "ChangeMeCreatedEvent"
	ChangeMeUpdatedEventName = "ChangeMeUpdatedEvent"
	ChangeMeDeletedEventName = "ChangeMeDeletedEvent"
)

type ChangeMeCreatedData struct {
	ID ChangeMeID
}

type ChangeMeCreatedEvent = ddd.Event[ChangeMeCreatedData]

func NewChangeMeCreatedEvent(data ChangeMeCreatedData) ChangeMeCreatedEvent {
	return ddd.NewEvent(ChangeMeCreatedEventName, data)
}

type ChangeMeUpdatedData struct {
	ID ChangeMeID
}

type ChangeMeUpdatedEvent = ddd.Event[ChangeMeUpdatedData]

func NewChangeMeUpdatedEvent(data ChangeMeUpdatedData) ChangeMeUpdatedEvent {
	return ddd.NewEvent(ChangeMeUpdatedEventName, data)
}

type ChangeMeDeletedData struct {
	ID ChangeMeID
}

type ChangeMeDeletedEvent = ddd.Event[ChangeMeDeletedData]

func NewChangeMeDeletedEvent(data ChangeMeDeletedData) ChangeMeDeletedEvent {
	return ddd.NewEvent(ChangeMeDeletedEventName, data)
}
