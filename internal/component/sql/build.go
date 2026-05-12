package sqlcomponent

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func buildCommandFromArgs(args []string) (Command, error) {
	fs := flag.NewFlagSet("sql", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	write := fs.Bool("write", false, "Allow a single mutating SQL statement")
	limit := fs.Int("limit", defaultRowLimit, "Maximum rows to print for read queries")
	if err := fs.Parse(args); err != nil {
		return Command{}, err
	}
	if *limit <= 0 {
		return Command{}, fmt.Errorf("limit must be positive")
	}
	if *limit > maxRowLimit {
		return Command{}, fmt.Errorf("limit must be <= %d", maxRowLimit)
	}

	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		content, err := io.ReadAll(os.Stdin)
		if err != nil {
			return Command{}, fmt.Errorf("read stdin: %w", err)
		}
		query = strings.TrimSpace(string(content))
	}
	return Command{Query: query, Write: *write, Limit: *limit}, nil
}
