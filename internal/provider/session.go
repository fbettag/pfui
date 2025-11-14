package provider

import (
	"fmt"
	"time"
)

type basicSession struct {
	id string
}

func (s *basicSession) ID() string {
	return s.id
}

func (s *basicSession) Close() error {
	return nil
}

// NewSession returns a lightweight session handle for providers that haven't implemented streaming yet.
func NewSession(prefix, requestedID string) Session {
	id := requestedID
	if id == "" {
		id = fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return &basicSession{id: id}
}
