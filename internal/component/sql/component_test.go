package sqlcomponent

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSQLCommandDefinitionIsHostbridgeOnly(t *testing.T) {
	definition := sqlCommandDefinition()
	if got := definition.CanonicalPattern(); got != "sql" {
		t.Fatalf("pattern = %q, want sql", got)
	}
	if definition.Help == "" {
		t.Fatal("help is empty")
	}
	if len(definition.Sources) != 1 || definition.Sources[0] != commandengine.SourceHostbridge {
		t.Fatalf("sources = %#v, want hostbridge only", definition.Sources)
	}
}

func TestSQLCommandDeniesWithoutChatID(t *testing.T) {
	engine, _ := newSQLEngine(t)

	_, err := engine.Run(context.Background(), agentRequest(modeluuid.Nil), []string{"sql", "SELECT 1"})
	if err == nil || !strings.Contains(err.Error(), "missing chat context") {
		t.Fatalf("Run(sql) error = %v, want missing chat context", err)
	}
}

func TestSQLSelectReturnsRowsWithLimit(t *testing.T) {
	engine, chatID := newSQLEngine(t)

	result, err := engine.Run(context.Background(), agentRequest(chatID), []string{"sql", "--limit", "1", "SELECT id, label FROM items ORDER BY id"})
	if err != nil {
		t.Fatalf("Run(sql select) error = %v", err)
	}
	if !strings.Contains(result.Text, "id\tlabel") || !strings.Contains(result.Text, "1\talpha") {
		t.Fatalf("select output = %q, want header and first row", result.Text)
	}
	if strings.Contains(result.Text, "beta") {
		t.Fatalf("select output = %q, want limited rows", result.Text)
	}
	if !strings.Contains(result.Text, "truncated at 1 rows") {
		t.Fatalf("select output = %q, want truncation notice", result.Text)
	}
}

func TestSQLWriteRequiresWriteFlag(t *testing.T) {
	engine, chatID := newSQLEngine(t)

	_, err := engine.Run(context.Background(), agentRequest(chatID), []string{"sql", "DELETE FROM items WHERE id = 1"})
	if err == nil || !strings.Contains(err.Error(), "requires --write") {
		t.Fatalf("Run(delete without --write) error = %v, want write requirement", err)
	}
}

func TestSQLWriteWithFlagExecutesSingleStatement(t *testing.T) {
	engine, chatID := newSQLEngine(t)

	result, err := engine.Run(context.Background(), agentRequest(chatID), []string{"sql", "--write", "DELETE FROM items WHERE id = 1"})
	if err != nil {
		t.Fatalf("Run(delete with --write) error = %v", err)
	}
	if !strings.Contains(result.Text, "rows_affected=1") {
		t.Fatalf("write output = %q, want rows affected", result.Text)
	}

	result, err = engine.Run(context.Background(), agentRequest(chatID), []string{"sql", "SELECT label FROM items ORDER BY id"})
	if err != nil {
		t.Fatalf("Run(select after delete) error = %v", err)
	}
	if strings.Contains(result.Text, "alpha") || !strings.Contains(result.Text, "beta") {
		t.Fatalf("select output after delete = %q", result.Text)
	}
}

func TestSQLRejectsMultipleStatements(t *testing.T) {
	engine, chatID := newSQLEngine(t)

	_, err := engine.Run(context.Background(), agentRequest(chatID), []string{"sql", "SELECT 1; SELECT 2"})
	if err == nil || !strings.Contains(err.Error(), "multiple SQL statements") {
		t.Fatalf("Run(multiple statements) error = %v, want rejection", err)
	}
}

func TestSQLCommandUsesStdinWhenQueryArgMissing(t *testing.T) {
	engine, chatID := newSQLEngine(t)

	err := runWithStdin(t, "SELECT label FROM items WHERE id = 2", func() error {
		result, err := engine.Run(context.Background(), agentRequest(chatID), []string{"sql"})
		if err != nil {
			return err
		}
		if !strings.Contains(result.Text, "beta") {
			t.Fatalf("stdin select output = %q, want beta", result.Text)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Run(sql from stdin) error = %v", err)
	}
}

func TestSQLCellOutputIsSafeAndTruncated(t *testing.T) {
	engine, chatID := newSQLEngine(t)

	result, err := engine.Run(context.Background(), agentRequest(chatID), []string{"sql", "SELECT note, payload FROM blobs"})
	if err != nil {
		t.Fatalf("Run(blob select) error = %v", err)
	}
	if !strings.Contains(result.Text, "line1\\nline2") {
		t.Fatalf("output = %q, want escaped newline", result.Text)
	}
	if !strings.Contains(result.Text, "<blob 3 bytes>") {
		t.Fatalf("output = %q, want blob placeholder", result.Text)
	}
	if strings.Contains(result.Text, strings.Repeat("x", maxCellLength+1)) {
		t.Fatalf("output = %q, want large value truncated", result.Text)
	}
	if !strings.Contains(result.Text, "…") {
		t.Fatalf("output = %q, want truncation marker", result.Text)
	}
}

func TestSQLCommandNotAvailableWithoutBoundSurface(t *testing.T) {
	engine, err := commandset.NewBoundEngineForSource(commandengine.SourceHostbridge, nil)
	if err != nil {
		t.Fatalf("NewBoundEngineForSource() error = %v", err)
	}
	if engine != nil {
		t.Fatalf("engine = %#v, want nil without bound surfaces", engine)
	}
}

func newSQLEngine(t *testing.T) (*commandengine.Engine, modeluuid.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, label TEXT)").Error; err != nil {
		t.Fatalf("create items: %v", err)
	}
	if err := db.Exec("INSERT INTO items (id, label) VALUES (1, 'alpha'), (2, 'beta')").Error; err != nil {
		t.Fatalf("insert items: %v", err)
	}
	long := strings.Repeat("x", maxCellLength+20)
	if err := db.Exec("CREATE TABLE blobs (note TEXT, payload BLOB)").Error; err != nil {
		t.Fatalf("create blobs: %v", err)
	}
	if err := db.Exec("INSERT INTO blobs (note, payload) VALUES (?, ?)", "line1\nline2 "+long, []byte{0xff, 0x00, 0x01}).Error; err != nil {
		t.Fatalf("insert blobs: %v", err)
	}

	component, err := New(db)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	engine, err := commandset.NewBoundEngineForSource(commandengine.SourceHostbridge, []commandset.BoundSurface{{
		Surface:       component,
		ComponentRef:  Type,
		ComponentType: Type,
	}})
	if err != nil {
		t.Fatalf("NewBoundEngineForSource() error = %v", err)
	}
	return engine, modeluuid.New()
}

func agentRequest(chatID modeluuid.UUID) commandengine.Request {
	return commandengine.Request{Context: commandengine.Context{
		Source: commandengine.SourceHostbridge,
		Actor:  commandengine.Actor{ID: "agent", Roles: []simplerbac.Role{simplerbac.RoleAgent}},
		ChatID: chatID,
	}}
}

func runWithStdin(t *testing.T, input string, fn func() error) error {
	t.Helper()
	old := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}
	if _, err := io.WriteString(writer, input); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}
	os.Stdin = reader
	defer func() {
		os.Stdin = old
		_ = reader.Close()
	}()
	return fn()
}
