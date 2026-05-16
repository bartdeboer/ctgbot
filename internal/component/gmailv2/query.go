package gmailv2

import (
	"context"
	"fmt"
	"strings"
)

const (
	defaultQueryLimit = 50
	maxQueryLimit     = 200
)

func (s *store) query(ctx context.Context, sql string) (string, error) {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return "", fmt.Errorf("missing query")
	}
	if err := validateSingleStatement(sql); err != nil {
		return "", err
	}
	if !isReadStatement(sql) {
		return "", fmt.Errorf("only SELECT queries are allowed")
	}
	if !hasLimit(sql) {
		sql = strings.TrimSuffix(sql, ";") + fmt.Sprintf(" LIMIT %d", defaultQueryLimit)
	}
	rows, err := s.db.WithContext(ctx).Raw(sql).Rows()
	if err != nil {
		return "", err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}
	var lines []string
	lines = append(lines, strings.Join(cols, "\t"))
	count := 0
	for rows.Next() {
		if count >= maxQueryLimit {
			break
		}
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return "", err
		}
		parts := make([]string, len(cols))
		for i, value := range values {
			parts[i] = cellText(value)
		}
		lines = append(lines, strings.Join(parts, "\t"))
		count++
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(lines) == 1 {
		return strings.Join(lines, "\n") + "\n(no rows)", nil
	}
	return strings.Join(lines, "\n"), nil
}

func validateSingleStatement(query string) error {
	semicolon := firstSemicolon(query)
	if semicolon < 0 {
		return nil
	}
	if hasNonWhitespaceAfter(query[semicolon+1:]) {
		return fmt.Errorf("multiple SQL statements are not allowed")
	}
	return nil
}

func isReadStatement(query string) bool {
	tokens := sqlTokens(query)
	if len(tokens) == 0 {
		return false
	}
	if tokens[0] == "select" {
		return true
	}
	if tokens[0] != "with" {
		return false
	}
	return withBodyIsSelect(tokens)
}

func withBodyIsSelect(tokens []string) bool {
	i := 1
	if i < len(tokens) && tokens[i] == "recursive" {
		i++
	}
	for i < len(tokens) {
		i++ // CTE name
		if i < len(tokens) && tokens[i] == "(" {
			i = skipBalancedTokens(tokens, i)
		}
		if i >= len(tokens) || tokens[i] != "as" {
			return false
		}
		i++
		if i < len(tokens) && tokens[i] == "not" {
			i++
		}
		if i < len(tokens) && tokens[i] == "materialized" {
			i++
		}
		if i >= len(tokens) || tokens[i] != "(" {
			return false
		}
		i = skipBalancedTokens(tokens, i)
		if i < len(tokens) && tokens[i] == "," {
			i++
			continue
		}
		return i < len(tokens) && tokens[i] == "select"
	}
	return false
}

func skipBalancedTokens(tokens []string, start int) int {
	depth := 0
	for i := start; i < len(tokens); i++ {
		switch tokens[i] {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(tokens)
}

func hasLimit(query string) bool {
	tokens := sqlTokens(query)
	depth := 0
	for _, token := range tokens {
		switch token {
		case "(":
			depth++
		case ")":
			if depth > 0 {
				depth--
			}
		case "limit":
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

func firstSemicolon(query string) int {
	inSingle, inDouble, inLineComment, inBlockComment := false, false, false, false
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && i+1 < len(query) && query[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inSingle {
			if ch == '\'' {
				if i+1 < len(query) && query[i+1] == '\'' {
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		if inDouble {
			if ch == '"' {
				if i+1 < len(query) && query[i+1] == '"' {
					i++
					continue
				}
				inDouble = false
			}
			continue
		}
		switch {
		case ch == '-' && i+1 < len(query) && query[i+1] == '-':
			inLineComment = true
			i++
		case ch == '/' && i+1 < len(query) && query[i+1] == '*':
			inBlockComment = true
			i++
		case ch == '\'':
			inSingle = true
		case ch == '"':
			inDouble = true
		case ch == ';':
			return i
		}
	}
	return -1
}

func hasNonWhitespaceAfter(s string) bool {
	for _, ch := range s {
		if ch != ' ' && ch != '\t' && ch != '\n' && ch != '\r' && ch != ';' {
			return true
		}
	}
	return false
}

func sqlTokens(query string) []string {
	var tokens []string
	inSingle, inDouble, inLineComment, inBlockComment := false, false, false, false
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && i+1 < len(query) && query[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inSingle {
			if ch == '\'' {
				if i+1 < len(query) && query[i+1] == '\'' {
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		if inDouble {
			if ch == '"' {
				if i+1 < len(query) && query[i+1] == '"' {
					i++
					continue
				}
				inDouble = false
			}
			continue
		}
		switch {
		case ch == '-' && i+1 < len(query) && query[i+1] == '-':
			inLineComment = true
			i++
		case ch == '/' && i+1 < len(query) && query[i+1] == '*':
			inBlockComment = true
			i++
		case ch == '\'':
			inSingle = true
		case ch == '"':
			inDouble = true
		case ch == '(' || ch == ')' || ch == ',':
			tokens = append(tokens, string(ch))
		case isIdentStart(ch):
			start := i
			for i+1 < len(query) && isIdentPart(query[i+1]) {
				i++
			}
			tokens = append(tokens, strings.ToLower(query[start:i+1]))
		}
	}
	return tokens
}

func isIdentStart(ch byte) bool {
	return ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9')
}

func cellText(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case []byte:
		return cleanCell(string(v))
	default:
		return cleanCell(fmt.Sprint(v))
	}
}

func cleanCell(value string) string {
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\t", "\\t")
	if len(value) > 300 {
		return value[:300] + "..."
	}
	return value
}
