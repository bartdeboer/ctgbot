package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, ""))
}

func run(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, cwd string) int {
	patch, exitCode, err := readPatchInput(args, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitCode
	}
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "Error: Failed to determine current directory.\n%v\n", err)
			return 1
		}
	}
	applier := Applier{Root: cwd}
	if err := applier.Apply(patch, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func readPatchInput(args []string, stdin io.Reader) (string, int, error) {
	switch len(args) {
	case 0:
		body, err := io.ReadAll(stdin)
		if err != nil {
			return "", 1, fmt.Errorf("Error: Failed to read PATCH from stdin.\n%v", err)
		}
		if len(body) == 0 {
			return "", 2, fmt.Errorf("Usage: apply_patch 'PATCH'\n       echo 'PATCH' | apply_patch")
		}
		return string(body), 0, nil
	case 1:
		return args[0], 0, nil
	default:
		return "", 2, fmt.Errorf("Error: apply_patch accepts exactly one argument.")
	}
}
