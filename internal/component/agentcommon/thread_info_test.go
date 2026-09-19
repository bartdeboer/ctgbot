package agentcommon

import (
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

func TestThreadInfoDBOnlyAndSessionReset(t *testing.T) {
	store := repository.NewMemory()
	thread := coremodel.Thread{ChatID: modeluuid.New()}
	_ = store.Threads().Save(t.Context(), &thread)
	c := &Core{Storage: store, Registration: coremodel.Component{ID: modeluuid.New()}} // nil Runtime must never be used.
	mapping := coremodel.ThreadComponentMapping{ThreadID: thread.ID, ChatID: thread.ChatID, ComponentID: c.Registration.ID, ComponentThreadID: "old"}
	_ = store.ThreadComponentMappings().Save(t.Context(), &mapping)
	zero := int64(0)
	m := coremodel.ThreadMessage{ThreadID: thread.ID, ComponentID: c.Registration.ID, IsFinal: true, Usage: coremodel.MessageUsage{ProviderSessionID: "old", InputTokens: &zero, Scope: "invocation"}, CreatedAt: time.Now().Add(-72 * time.Hour)}
	_ = store.Messages().Append(t.Context(), &m)
	req := commandengine.Request{Context: commandengine.Context{ThreadID: thread.ID}}
	got, err := c.threadInfo(t.Context(), req, "test")
	if err != nil || !strings.Contains(got.Text, "Input (including cache): 0") || !strings.Contains(got.Text, "Output: unavailable") {
		t.Fatal(got, err)
	}
	mapping.ComponentThreadID = "new"
	_ = store.ThreadComponentMappings().Save(t.Context(), &mapping)
	got, err = c.threadInfo(t.Context(), req, "test")
	if err != nil || !strings.Contains(got.Text, "No final-response") {
		t.Fatal(got, err)
	}
	_ = store.ThreadComponentMappings().DeleteByThreadAndComponent(t.Context(), thread.ID, c.Registration.ID)
	got, err = c.threadInfo(t.Context(), req, "test")
	if err != nil || !strings.Contains(got.Text, "No final-response") {
		t.Fatal(got, err)
	}
}
