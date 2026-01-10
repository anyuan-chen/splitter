package core

import (
	"separate/server/models"
)

// ProgressBroadcaster manages SSE client subscriptions
type ProgressBroadcaster struct {
	events         chan models.ProgressEvent
	newClients     chan chan models.ProgressEvent
	closingClients chan chan models.ProgressEvent
	clients        map[chan models.ProgressEvent]bool
}

// NewProgressBroadcaster creates and starts a new progress broadcaster
func NewProgressBroadcaster() *ProgressBroadcaster {
	b := &ProgressBroadcaster{
		events:         make(chan models.ProgressEvent, 100), // Buffered for bursts of progress updates
		newClients:     make(chan chan models.ProgressEvent),
		closingClients: make(chan chan models.ProgressEvent),
		clients:        make(map[chan models.ProgressEvent]bool),
	}
	go b.run()
	return b
}

func (b *ProgressBroadcaster) run() {
	for {
		select {
		case client := <-b.newClients:
			b.clients[client] = true
		case client := <-b.closingClients:
			delete(b.clients, client)
			close(client)
		case event := <-b.events:
			// Broadcast to all clients
			for client := range b.clients {
				select {
				case client <- event:
				default:
					// Client is slow/blocked, skip
				}
			}
		}
	}
}

// SendEvent broadcasts a progress event to all connected clients
func (b *ProgressBroadcaster) SendEvent(event models.ProgressEvent) {
	b.events <- event
}

// Events returns the channel used to send events (useful for workers)
func (b *ProgressBroadcaster) Events() chan models.ProgressEvent {
	return b.events
}

// RegisterClient registers a new client and returns their channel
func (b *ProgressBroadcaster) RegisterClient() chan models.ProgressEvent {
	clientChan := make(chan models.ProgressEvent)
	b.newClients <- clientChan
	return clientChan
}

// UnregisterClient unregisters a client
func (b *ProgressBroadcaster) UnregisterClient(clientChan chan models.ProgressEvent) {
	b.closingClients <- clientChan
}
