package core

import (
	"separate/server/models"
)

// clientInfo holds a client channel and optional filter for playlist-specific subscriptions
type clientInfo struct {
	channel       chan models.ProgressEvent
	trackIDFilter map[string]bool // nil means no filter (receive all events)
}

// clientRegistration represents a new client registration request
type clientRegistration struct {
	channel       chan models.ProgressEvent
	trackIDFilter map[string]bool
}

// ProgressBroadcaster manages SSE client subscriptions
type ProgressBroadcaster struct {
	events         chan models.ProgressEvent
	newClients     chan clientRegistration
	closingClients chan chan models.ProgressEvent
	clients        map[chan models.ProgressEvent]*clientInfo
}

// NewProgressBroadcaster creates and starts a new progress broadcaster
func NewProgressBroadcaster() *ProgressBroadcaster {
	b := &ProgressBroadcaster{
		events:         make(chan models.ProgressEvent, 100), // Buffered for bursts of progress updates
		newClients:     make(chan clientRegistration),
		closingClients: make(chan chan models.ProgressEvent),
		clients:        make(map[chan models.ProgressEvent]*clientInfo),
	}
	go b.run()
	return b
}

func (b *ProgressBroadcaster) run() {
	for {
		select {
		case registration := <-b.newClients:
			b.clients[registration.channel] = &clientInfo{
				channel:       registration.channel,
				trackIDFilter: registration.trackIDFilter,
			}
		case clientChan := <-b.closingClients:
			delete(b.clients, clientChan)
			close(clientChan)
		case event := <-b.events:
			// Broadcast to all clients that match the filter
			for _, client := range b.clients {
				// Check if client has a filter and if so, whether this event matches
				if client.trackIDFilter != nil && !client.trackIDFilter[event.TrackID] {
					continue // Skip this client, event doesn't match their filter
				}
				select {
				case client.channel <- event:
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

// RegisterClient registers a new client with optional playlist filtering
// If trackIDFilter is nil, the client receives all events
// If trackIDFilter is provided, the client only receives events for those track IDs
func (b *ProgressBroadcaster) RegisterClient(trackIDFilter map[string]bool) chan models.ProgressEvent {
	clientChan := make(chan models.ProgressEvent)
	b.newClients <- clientRegistration{
		channel:       clientChan,
		trackIDFilter: trackIDFilter,
	}
	return clientChan
}

// UnregisterClient unregisters a client
func (b *ProgressBroadcaster) UnregisterClient(clientChan chan models.ProgressEvent) {
	b.closingClients <- clientChan
}
