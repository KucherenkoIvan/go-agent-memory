package domain

import "github.com/KucherenkoIvan/go-kernel/ddd"

const (
	MemoryStoredEventName     = "MemoryStoredEvent"
	MemoryRatedEventName      = "MemoryRatedEvent"
	MemorySupersededEventName = "MemorySupersededEvent"
	MemoryDeletedEventName    = "MemoryDeletedEvent"
)

type MemoryStoredData struct {
	MemoryID MemoryID
	Kind     Kind
	Keywords []string
}

type MemoryStoredEvent = ddd.Event[MemoryStoredData]

func NewMemoryStoredEvent(data MemoryStoredData) MemoryStoredEvent {
	return ddd.NewEvent(MemoryStoredEventName, data)
}

type MemoryRatedData struct {
	MemoryID MemoryID
	Up       bool
}

type MemoryRatedEvent = ddd.Event[MemoryRatedData]

func NewMemoryRatedEvent(data MemoryRatedData) MemoryRatedEvent {
	return ddd.NewEvent(MemoryRatedEventName, data)
}

type MemorySupersededData struct {
	MemoryID MemoryID
	By       MemoryID
}

type MemorySupersededEvent = ddd.Event[MemorySupersededData]

func NewMemorySupersededEvent(data MemorySupersededData) MemorySupersededEvent {
	return ddd.NewEvent(MemorySupersededEventName, data)
}

type MemoryDeletedData struct {
	MemoryID MemoryID
}

type MemoryDeletedEvent = ddd.Event[MemoryDeletedData]

func NewMemoryDeletedEvent(data MemoryDeletedData) MemoryDeletedEvent {
	return ddd.NewEvent(MemoryDeletedEventName, data)
}
