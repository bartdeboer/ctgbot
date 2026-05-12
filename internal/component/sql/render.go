package sqlcomponent

import (
	databasesql "database/sql"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxCellLength = 240

func renderRows(rows *databasesql.Rows, limit int) (string, error) {
	columns, err := rows.Columns()
	if err != nil {
		return "", err
	}
	var out strings.Builder
	out.WriteString(strings.Join(columns, "\t"))

	count := 0
	truncated := false
	for rows.Next() {
		if count >= limit {
			truncated = true
			break
		}
		values := make([]any, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return "", err
		}
		out.WriteByte('\n')
		for i, value := range values {
			if i > 0 {
				out.WriteByte('\t')
			}
			out.WriteString(formatCell(value))
		}
		count++
	}
	if count == 0 {
		out.WriteString("\n(no rows)")
	}
	if truncated {
		out.WriteString(fmt.Sprintf("\n... truncated at %d rows", limit))
	}
	return out.String(), nil
}

func formatCell(value any) string {
	switch v := value.(type) {
	case nil:
		return "NULL"
	case []byte:
		return formatBytes(v)
	case string:
		return truncateCell(sanitizeCell(v))
	default:
		return truncateCell(sanitizeCell(fmt.Sprint(v)))
	}
}

func formatBytes(value []byte) string {
	if !utf8.Valid(value) || !mostlyPrintable(string(value)) {
		return fmt.Sprintf("<blob %d bytes>", len(value))
	}
	return truncateCell(sanitizeCell(string(value)))
}

func mostlyPrintable(value string) bool {
	for _, r := range value {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func sanitizeCell(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, "\t", `\t`)
	return value
}

func truncateCell(value string) string {
	if len([]rune(value)) <= maxCellLength {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxCellLength]) + "…"
}
