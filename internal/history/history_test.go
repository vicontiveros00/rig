package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestDirs(t *testing.T) (cleanup func()) {
	t.Helper()
	dir := t.TempDir()

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)

	os.MkdirAll(filepath.Join(dir, ".rig", "history", "chat"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".rig", "history", "scratch"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".rig", "history", "plan"), 0o755)

	return func() { os.Setenv("HOME", origHome) }
}

func TestSaveAndLoadChat(t *testing.T) {
	cleanup := setupTestDirs(t)
	defer cleanup()

	session := ChatSession{
		ID:        "2026-01-01T00-00-00_test-model",
		Provider:  "openai",
		Model:     "gpt-4o",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages: []MessageRecord{
			{Role: "user", Content: "hello", Timestamp: time.Now()},
			{Role: "assistant", Content: "hi there", Timestamp: time.Now()},
		},
	}

	if err := SaveChat(session); err != nil {
		t.Fatalf("SaveChat failed: %v", err)
	}

	loaded, err := LoadChat(session.ID)
	if err != nil {
		t.Fatalf("LoadChat failed: %v", err)
	}

	if loaded.ID != session.ID {
		t.Errorf("expected ID=%s, got %s", session.ID, loaded.ID)
	}
	if loaded.Model != "gpt-4o" {
		t.Errorf("expected model=gpt-4o, got %s", loaded.Model)
	}
	if len(loaded.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(loaded.Messages))
	}
	if loaded.Messages[0].Content != "hello" {
		t.Errorf("expected first message=hello, got %s", loaded.Messages[0].Content)
	}
}

func TestListChats(t *testing.T) {
	cleanup := setupTestDirs(t)
	defer cleanup()

	for i, id := range []string{"2026-01-01T00-00-00_a", "2026-01-02T00-00-00_b"} {
		session := ChatSession{
			ID:        id,
			Model:     "model",
			CreatedAt: time.Now().Add(time.Duration(i) * time.Hour),
			Messages:  []MessageRecord{{Role: "user", Content: "msg " + id}},
		}
		SaveChat(session)
	}

	metas, err := ListChats()
	if err != nil {
		t.Fatalf("ListChats failed: %v", err)
	}
	if len(metas) != 2 {
		t.Fatalf("expected 2 chats, got %d", len(metas))
	}
	// Should be sorted newest first
	if metas[0].CreatedAt.Before(metas[1].CreatedAt) {
		t.Error("expected newest first")
	}
}

func TestDeleteChat(t *testing.T) {
	cleanup := setupTestDirs(t)
	defer cleanup()

	session := ChatSession{ID: "delete-me", Model: "m", CreatedAt: time.Now()}
	SaveChat(session)

	if err := DeleteChat("delete-me"); err != nil {
		t.Fatalf("DeleteChat failed: %v", err)
	}

	_, err := LoadChat("delete-me")
	if err == nil {
		t.Error("expected error loading deleted chat")
	}
}

func TestArchiveAndListScratches(t *testing.T) {
	cleanup := setupTestDirs(t)
	defer cleanup()

	if err := ArchiveScratch("test content\nline two"); err != nil {
		t.Fatalf("ArchiveScratch failed: %v", err)
	}

	metas, err := ListScratches()
	if err != nil {
		t.Fatalf("ListScratches failed: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("expected 1 scratch, got %d", len(metas))
	}
	if metas[0].Preview != "test content" {
		t.Errorf("expected preview='test content', got %q", metas[0].Preview)
	}
}

func TestArchiveScratchEmpty(t *testing.T) {
	cleanup := setupTestDirs(t)
	defer cleanup()

	if err := ArchiveScratch(""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ArchiveScratch("   "); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	metas, _ := ListScratches()
	if len(metas) != 0 {
		t.Errorf("expected 0 scratches for empty content, got %d", len(metas))
	}
}

func TestSaveAndLoadPlan(t *testing.T) {
	cleanup := setupTestDirs(t)
	defer cleanup()

	plan := Plan{
		ID:        "2026-01-01T00-00-00_test-plan",
		Title:     "Test Plan",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Tasks: []Task{
			{ID: "t1", Title: "first task", Status: "pending"},
			{ID: "t2", Title: "second task", Status: "done", Notes: "completed"},
		},
	}

	if err := SavePlan(plan); err != nil {
		t.Fatalf("SavePlan failed: %v", err)
	}

	loaded, err := LoadPlan(plan.ID)
	if err != nil {
		t.Fatalf("LoadPlan failed: %v", err)
	}

	if loaded.Title != "Test Plan" {
		t.Errorf("expected title='Test Plan', got %s", loaded.Title)
	}
	if len(loaded.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(loaded.Tasks))
	}
	if loaded.Tasks[1].Notes != "completed" {
		t.Errorf("expected notes='completed', got %s", loaded.Tasks[1].Notes)
	}
}

func TestListPlans(t *testing.T) {
	cleanup := setupTestDirs(t)
	defer cleanup()

	plan := Plan{
		ID:        "test-plan-1",
		Title:     "Plan A",
		CreatedAt: time.Now(),
		Tasks: []Task{
			{ID: "t1", Status: "done"},
			{ID: "t2", Status: "pending"},
			{ID: "t3", Status: "pending", Children: []Task{{ID: "t4", Status: "done"}}},
		},
	}
	SavePlan(plan)

	metas, err := ListPlans()
	if err != nil {
		t.Fatalf("ListPlans failed: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(metas))
	}
	if metas[0].TaskCount != 4 {
		t.Errorf("expected 4 tasks (including children), got %d", metas[0].TaskCount)
	}
	if metas[0].DoneCount != 2 {
		t.Errorf("expected 2 done, got %d", metas[0].DoneCount)
	}
}

func TestSetAndGetActivePlan(t *testing.T) {
	cleanup := setupTestDirs(t)
	defer cleanup()

	if err := SetActivePlan("my-plan-id"); err != nil {
		t.Fatalf("SetActivePlan failed: %v", err)
	}

	id, err := GetActivePlan()
	if err != nil {
		t.Fatalf("GetActivePlan failed: %v", err)
	}
	if id != "my-plan-id" {
		t.Errorf("expected 'my-plan-id', got %q", id)
	}
}

func TestGetActivePlanEmpty(t *testing.T) {
	cleanup := setupTestDirs(t)
	defer cleanup()

	id, err := GetActivePlan()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "" {
		t.Errorf("expected empty, got %q", id)
	}
}

func TestGenerateChatID(t *testing.T) {
	id := GenerateChatID("gpt-4o")
	if id == "" {
		t.Error("expected non-empty ID")
	}
	if len(id) < 20 {
		t.Errorf("ID seems too short: %s", id)
	}
}

func TestGeneratePlanID(t *testing.T) {
	id := GeneratePlanID("my cool plan")
	if id == "" {
		t.Error("expected non-empty ID")
	}
	if len(id) < 20 {
		t.Errorf("ID seems too short: %s", id)
	}
}
