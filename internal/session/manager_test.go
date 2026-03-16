package session

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStore_CreateAndGet(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	s := &Session{
		ID:        "sess-1",
		UserID:    "user-1",
		IPAddress: "10.0.0.1",
		UserAgent: "test-agent",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	if err := store.Create(ctx, s); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	got, err := store.Get(ctx, "sess-1")
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if got.ID != s.ID {
		t.Errorf("Get: got ID %q, want %q", got.ID, s.ID)
	}
	if got.UserID != s.UserID {
		t.Errorf("Get: got UserID %q, want %q", got.UserID, s.UserID)
	}
}

func TestMemoryStore_GetNotFound(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	_, err := store.Get(ctx, "nonexistent")
	if err == nil {
		t.Fatal("Get: expected error for nonexistent session, got nil")
	}
}

func TestMemoryStore_GetExpiredSession(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	s := &Session{
		ID:        "sess-expired",
		UserID:    "user-1",
		CreatedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour), // already expired
	}

	if err := store.Create(ctx, s); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	_, err := store.Get(ctx, "sess-expired")
	if err == nil {
		t.Fatal("Get: expected error for expired session, got nil")
	}

	// Verify session was cleaned up from store
	_, err = store.Get(ctx, "sess-expired")
	if err == nil {
		t.Fatal("Get: expired session should have been deleted from store")
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	s := &Session{
		ID:        "sess-del",
		UserID:    "user-1",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	if err := store.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.Delete(ctx, "sess-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := store.Get(ctx, "sess-del")
	if err == nil {
		t.Fatal("Get after Delete: expected error, got nil")
	}
}

func TestMemoryStore_DeleteNonexistent(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Deleting a nonexistent session should not error (sync.Map.Delete is a no-op).
	if err := store.Delete(ctx, "does-not-exist"); err != nil {
		t.Fatalf("Delete nonexistent: unexpected error: %v", err)
	}
}

func TestMemoryStore_DeleteByUser(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	sessions := []*Session{
		{ID: "s1", UserID: "user-a", ExpiresAt: time.Now().Add(time.Hour)},
		{ID: "s2", UserID: "user-a", ExpiresAt: time.Now().Add(time.Hour)},
		{ID: "s3", UserID: "user-b", ExpiresAt: time.Now().Add(time.Hour)},
	}
	for _, s := range sessions {
		if err := store.Create(ctx, s); err != nil {
			t.Fatalf("Create %s: %v", s.ID, err)
		}
	}

	if err := store.DeleteByUser(ctx, "user-a"); err != nil {
		t.Fatalf("DeleteByUser: %v", err)
	}

	// user-a sessions should be gone
	if _, err := store.Get(ctx, "s1"); err == nil {
		t.Error("s1 should have been deleted for user-a")
	}
	if _, err := store.Get(ctx, "s2"); err == nil {
		t.Error("s2 should have been deleted for user-a")
	}

	// user-b session should remain
	if _, err := store.Get(ctx, "s3"); err != nil {
		t.Errorf("s3 should still exist for user-b: %v", err)
	}
}

func TestMemoryStore_List(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	sessions := []*Session{
		{ID: "s1", UserID: "user-a", ExpiresAt: time.Now().Add(time.Hour)},
		{ID: "s2", UserID: "user-a", ExpiresAt: time.Now().Add(time.Hour)},
		{ID: "s3", UserID: "user-b", ExpiresAt: time.Now().Add(time.Hour)},
		{ID: "s4", UserID: "user-a", ExpiresAt: time.Now().Add(-time.Hour)}, // expired
	}
	for _, s := range sessions {
		if err := store.Create(ctx, s); err != nil {
			t.Fatalf("Create %s: %v", s.ID, err)
		}
	}

	list, err := store.List(ctx, "user-a")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("List: got %d sessions, want 2 (expired should be excluded)", len(list))
	}

	// Verify none of the returned sessions belong to user-b
	for _, s := range list {
		if s.UserID != "user-a" {
			t.Errorf("List: returned session %s for user %s, want user-a", s.ID, s.UserID)
		}
	}
}

func TestMemoryStore_ListEmpty(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	list, err := store.List(ctx, "nobody")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list != nil && len(list) != 0 {
		t.Errorf("List: expected empty slice, got %d items", len(list))
	}
}

func TestMemoryStore_Touch(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	original := time.Now().Add(30 * time.Minute)
	s := &Session{
		ID:        "sess-touch",
		UserID:    "user-1",
		ExpiresAt: original,
	}

	if err := store.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newTTL := 2 * time.Hour
	if err := store.Touch(ctx, "sess-touch", newTTL); err != nil {
		t.Fatalf("Touch: %v", err)
	}

	got, err := store.Get(ctx, "sess-touch")
	if err != nil {
		t.Fatalf("Get after Touch: %v", err)
	}

	if !got.ExpiresAt.After(original) {
		t.Error("Touch: ExpiresAt should have been extended beyond original")
	}
}

func TestMemoryStore_TouchNotFound(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	err := store.Touch(ctx, "nonexistent", time.Hour)
	if err == nil {
		t.Fatal("Touch: expected error for nonexistent session, got nil")
	}
}

func TestSession_isExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"future", time.Now().Add(time.Hour), false},
		{"past", time.Now().Add(-time.Hour), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{ExpiresAt: tt.expiresAt}
			if got := s.isExpired(); got != tt.want {
				t.Errorf("isExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateSessionID(t *testing.T) {
	id, err := generateSessionID()
	if err != nil {
		t.Fatalf("generateSessionID: %v", err)
	}
	// 32 bytes = 64 hex characters
	if len(id) != 64 {
		t.Errorf("generateSessionID: got length %d, want 64", len(id))
	}

	// IDs should be unique
	id2, err := generateSessionID()
	if err != nil {
		t.Fatalf("generateSessionID (2nd call): %v", err)
	}
	if id == id2 {
		t.Error("generateSessionID: two calls produced the same ID")
	}
}
