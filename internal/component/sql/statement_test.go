package sqlcomponent

import "testing"

func TestSQLStatementValidationAllowsSemicolonInString(t *testing.T) {
	if err := validateSingleStatement("SELECT ';' AS semi;"); err != nil {
		t.Fatalf("validateSingleStatement() error = %v", err)
	}
}

func TestSQLStatementKindRecognizesWithSelect(t *testing.T) {
	query := "WITH recent AS (SELECT id FROM items) SELECT id FROM recent"
	if !isReadStatement(query) {
		t.Fatalf("isReadStatement(%q) = false, want true", query)
	}
}

func TestSQLStatementKindRejectsWithDelete(t *testing.T) {
	query := "WITH recent AS (SELECT id FROM items) DELETE FROM items WHERE id IN (SELECT id FROM recent)"
	if isReadStatement(query) {
		t.Fatalf("isReadStatement(%q) = true, want false", query)
	}
}
