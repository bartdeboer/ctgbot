package toolloop

import "testing"

func TestConversationStoreSavesAndLoadsConversation(t *testing.T) {
	t.Parallel()
	store := NewConversationStore(t.TempDir())
	conversation := store.New()
	conversation.Model = "qwen"
	conversation.Messages = []Message{{Role: "user", Content: "hello"}}
	if err := store.Save(conversation); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	loaded, err := store.Load(conversation.ID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.ID != conversation.ID || loaded.Model != "qwen" || len(loaded.Messages) != 1 {
		t.Fatalf("loaded = %#v", loaded)
	}
}
