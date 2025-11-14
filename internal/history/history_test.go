package history

import "testing"

func TestCreateAndListSession(t *testing.T) {
	dir := t.TempDir()
	overrideHistoryPath(t, dir)

	project := "/tmp/project"
	sess, err := CreateSession(project)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.Project != project {
		t.Fatalf("expected project %q, got %q", project, sess.Project)
	}

	sessions, err := List(project)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
}

func TestSaveUpdatesSummary(t *testing.T) {
	dir := t.TempDir()
	overrideHistoryPath(t, dir)

	project := "/tmp/project"
	sess, err := CreateSession(project)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sess.Summary = "updated"
	if err := Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Get(sess.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if loaded.Summary != "updated" {
		t.Fatalf("expected summary 'updated', got %q", loaded.Summary)
	}
}

func overrideHistoryPath(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PFUI_HOME", dir)
}
