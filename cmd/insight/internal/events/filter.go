package events

// AllowList checks if an event type is allowed.
// Empty allowlist means all events are allowed.
type AllowList map[string]struct{}

// NewAllowList creates an allowlist from event type names.
// If the input is nil or empty, returns nil (allow-all mode).
func NewAllowList(events []string) AllowList {
	if len(events) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(events))
	for _, e := range events {
		set[e] = struct{}{}
	}

	return set
}

// Allows returns true if the event type is allowed
// or the list is nil (allow-all mode).
func (a AllowList) Allows(eventType string) bool {
	if a == nil {
		return true
	}

	_, ok := a[eventType]

	return ok
}
