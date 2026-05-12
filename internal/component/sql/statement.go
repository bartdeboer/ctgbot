package sqlcomponent

import (
	"fmt"
	"strings"
	"unicode"
)

func validateSingleStatement(query string) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return fmt.Errorf("missing SQL query")
	}
	semicolon := firstStatementSemicolon(query)
	if semicolon < 0 {
		return nil
	}
	if hasStatementAfterTerminator(query[semicolon+1:]) {
		return fmt.Errorf("multiple SQL statements are not allowed")
	}
	return nil
}

func isReadStatement(query string) bool {
	return mainStatementKind(query) == "select"
}

func mainStatementKind(query string) string {
	tokens := sqlTokens(query)
	if len(tokens) == 0 {
		return ""
	}
	if tokens[0] != "with" {
		return tokens[0]
	}
	return withMainStatementKind(tokens)
}

func withMainStatementKind(tokens []string) string {
	i := 1
	if i < len(tokens) && tokens[i] == "recursive" {
		i++
	}
	for i < len(tokens) {
		i++ // CTE name.
		if i < len(tokens) && tokens[i] == "(" {
			i = skipBalanced(tokens, i)
		}
		if i >= len(tokens) || tokens[i] != "as" {
			return "with"
		}
		i++
		if i+1 < len(tokens) && tokens[i] == "not" && tokens[i+1] == "materialized" {
			i += 2
		} else if i < len(tokens) && tokens[i] == "materialized" {
			i++
		}
		if i >= len(tokens) || tokens[i] != "(" {
			return "with"
		}
		i = skipBalanced(tokens, i)
		if i < len(tokens) && tokens[i] == "," {
			i++
			continue
		}
		if i < len(tokens) {
			return tokens[i]
		}
	}
	return "with"
}

func skipBalanced(tokens []string, start int) int {
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

func firstStatementSemicolon(query string) int {
	state := sqlScanState{}
	for i := 0; i < len(query); i++ {
		if state.consume(query, &i) {
			continue
		}
		if query[i] == ';' {
			return i
		}
	}
	return -1
}

func hasStatementAfterTerminator(rest string) bool {
	for i := 0; i < len(rest); i++ {
		r := rune(rest[i])
		if unicode.IsSpace(r) || rest[i] == ';' {
			continue
		}
		return true
	}
	return false
}

func sqlTokens(query string) []string {
	var tokens []string
	state := sqlScanState{}
	for i := 0; i < len(query); i++ {
		if state.consume(query, &i) {
			continue
		}
		ch := query[i]
		switch {
		case ch == '(' || ch == ')' || ch == ',':
			tokens = append(tokens, string(ch))
		case isIdentifierStart(ch):
			start := i
			for i+1 < len(query) && isIdentifierPart(query[i+1]) {
				i++
			}
			tokens = append(tokens, strings.ToLower(query[start:i+1]))
		}
	}
	return tokens
}

type sqlScanState struct {
	singleQuote  bool
	doubleQuote  bool
	lineComment  bool
	blockComment bool
}

func (s *sqlScanState) consume(query string, i *int) bool {
	ch := query[*i]
	if s.lineComment {
		if ch == '\n' {
			s.lineComment = false
		}
		return true
	}
	if s.blockComment {
		if ch == '*' && *i+1 < len(query) && query[*i+1] == '/' {
			s.blockComment = false
			(*i)++
		}
		return true
	}
	if s.singleQuote {
		if ch == '\'' {
			if *i+1 < len(query) && query[*i+1] == '\'' {
				(*i)++
				return true
			}
			s.singleQuote = false
		}
		return true
	}
	if s.doubleQuote {
		if ch == '"' {
			if *i+1 < len(query) && query[*i+1] == '"' {
				(*i)++
				return true
			}
			s.doubleQuote = false
		}
		return true
	}
	if ch == '-' && *i+1 < len(query) && query[*i+1] == '-' {
		s.lineComment = true
		(*i)++
		return true
	}
	if ch == '/' && *i+1 < len(query) && query[*i+1] == '*' {
		s.blockComment = true
		(*i)++
		return true
	}
	if ch == '\'' {
		s.singleQuote = true
		return true
	}
	if ch == '"' {
		s.doubleQuote = true
		return true
	}
	return false
}

func isIdentifierStart(ch byte) bool {
	return ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isIdentifierPart(ch byte) bool {
	return isIdentifierStart(ch) || (ch >= '0' && ch <= '9')
}
