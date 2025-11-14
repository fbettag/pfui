package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

// Session represents a persisted chat.
type Session struct {
	ID        string    `json:"id"`
	Project   string    `json:"project"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const historyFile = "history.json"

// CreateSession registers a new session for the given project.
func CreateSession(project string) (Session, error) {
	s := Session{
		ID:        uuid.New().String(),
		Project:   project,
		Title:     "New chat",
		Summary:   "",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := saveSession(s); err != nil {
		return Session{}, err
	}
	return s, nil
}

// Save updates the stored session metadata.
func Save(session Session) error {
	session.UpdatedAt = time.Now().UTC()
	return saveSession(session)
}

// Get returns a session by ID.
func Get(id string) (Session, error) {
	store, err := loadStore()
	if err != nil {
		return Session{}, err
	}
	s, ok := store[id]
	if !ok {
		return Session{}, fmt.Errorf("session %s not found", id)
	}
	return s, nil
}

// List returns sessions filtered by project (empty project means all).
func List(project string) ([]Session, error) {
	store, err := loadStore()
	if err != nil {
		return nil, err
	}
	var sessions []Session
	for _, session := range store {
		if project == "" || session.Project == project {
			sessions = append(sessions, session)
		}
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

func saveSession(s Session) error {
	store, err := loadStore()
	if err != nil {
		return err
	}
	store[s.ID] = s
	return writeStore(store)
}

func loadStore() (map[string]Session, error) {
	path, err := historyPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]Session), nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading history: %w", err)
	}
	if len(data) == 0 {
		return make(map[string]Session), nil
	}
	var store map[string]Session
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("parsing history: %w", err)
	}
	return store, nil
}

func writeStore(store map[string]Session) error {
	path, err := historyPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding history: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing history: %w", err)
	}
	return nil
}

func historyPath() (string, error) {
	if custom := os.Getenv("PFUI_HOME"); custom != "" {
		if err := os.MkdirAll(custom, 0o755); err != nil {
			return "", fmt.Errorf("ensuring PFUI_HOME dir: %w", err)
		}
		return filepath.Join(custom, historyFile), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	dir := filepath.Join(home, ".pfui")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("ensuring history dir: %w", err)
	}
	return filepath.Join(dir, historyFile), nil
}
