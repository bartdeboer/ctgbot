package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type options struct {
	stateRoot string
	apply     bool
}

type summary struct {
	Threads    int
	Homes      int
	Copied     int
	Skipped    int
	Conflicts  int
	WouldCopy  int
	WouldSkip  int
	WouldMake  int
	MakeDirs   int
	ThreadLogs []string
}

func main() {
	code := run(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(code)
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	var opts options
	fs := flag.NewFlagSet("migratehomes", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.stateRoot, "state-root", "", "ctgbot state root containing threads/")
	fs.BoolVar(&opts.apply, "apply", false, "copy files; without this flag migratehomes only reports what would happen")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(opts.stateRoot) == "" {
		fmt.Fprintln(stderr, "error: missing --state-root")
		usage(stderr)
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "error: unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		usage(stderr)
		return 2
	}

	sum, err := migrate(opts, stdout)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	printSummary(stdout, opts, sum)
	if sum.Conflicts > 0 {
		return 3
	}
	return 0
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: migratehomes --state-root <path> [--apply]")
}

func migrate(opts options, stdout io.Writer) (summary, error) {
	root, err := filepath.Abs(opts.stateRoot)
	if err != nil {
		return summary{}, err
	}
	threadsDir := filepath.Join(root, "threads")
	entries, err := os.ReadDir(threadsDir)
	if err != nil {
		return summary{}, fmt.Errorf("read threads dir %s: %w", threadsDir, err)
	}

	var sum summary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		threadID := entry.Name()
		threadDir := filepath.Join(threadsDir, threadID)
		oldHomes, err := findOldHomes(threadDir)
		if err != nil {
			return sum, err
		}
		if len(oldHomes) == 0 {
			continue
		}
		sum.Threads++
		sum.Homes += len(oldHomes)
		target := filepath.Join(threadDir, "home")
		fmt.Fprintf(stdout, "thread %s:\n", threadID)
		for _, oldHome := range oldHomes {
			rel, _ := filepath.Rel(threadDir, oldHome)
			fmt.Fprintf(stdout, "  %s -> home\n", filepath.ToSlash(rel))
			result, err := copyTree(copyOptions{Source: oldHome, Target: target, Apply: opts.apply})
			if err != nil {
				return sum, fmt.Errorf("copy %s -> %s: %w", oldHome, target, err)
			}
			sum.Copied += result.Copied
			sum.Skipped += result.Skipped
			sum.Conflicts += result.Conflicts
			sum.WouldCopy += result.WouldCopy
			sum.WouldSkip += result.WouldSkip
			sum.WouldMake += result.WouldMake
			sum.MakeDirs += result.MakeDirs
			for _, conflict := range result.ConflictPaths {
				fmt.Fprintf(stdout, "    conflict: %s\n", filepath.ToSlash(conflict))
			}
		}
	}
	return sum, nil
}

func findOldHomes(threadDir string) ([]string, error) {
	componentsDir := filepath.Join(threadDir, "components")
	componentTypes, err := os.ReadDir(componentsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read components dir %s: %w", componentsDir, err)
	}
	var homes []string
	for _, componentType := range componentTypes {
		if !componentType.IsDir() {
			continue
		}
		typeDir := filepath.Join(componentsDir, componentType.Name())
		components, err := os.ReadDir(typeDir)
		if err != nil {
			return nil, fmt.Errorf("read component type dir %s: %w", typeDir, err)
		}
		for _, component := range components {
			if !component.IsDir() {
				continue
			}
			home := filepath.Join(typeDir, component.Name(), "home")
			info, err := os.Stat(home)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("stat old home %s: %w", home, err)
			}
			if info.IsDir() {
				homes = append(homes, home)
			}
		}
	}
	sort.Strings(homes)
	return homes, nil
}

type copyOptions struct {
	Source string
	Target string
	Apply  bool
}

type copyResult struct {
	Copied        int
	Skipped       int
	Conflicts     int
	WouldCopy     int
	WouldSkip     int
	WouldMake     int
	MakeDirs      int
	ConflictPaths []string
}

func copyTree(opts copyOptions) (copyResult, error) {
	var result copyResult
	err := filepath.WalkDir(opts.Source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(opts.Source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			if opts.Apply {
				if err := os.MkdirAll(opts.Target, 0o755); err != nil {
					return err
				}
				result.MakeDirs++
			} else {
				result.WouldMake++
			}
			return nil
		}
		target := filepath.Join(opts.Target, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return copyDir(target, rel, info, opts.Apply, &result)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return copySymlink(path, target, rel, opts.Apply, &result)
		}
		if info.Mode().IsRegular() {
			return copyFile(path, target, rel, info, opts.Apply, &result)
		}
		return handleSpecial(target, rel, &result)
	})
	return result, err
}

func copyDir(target string, rel string, info fs.FileInfo, apply bool, result *copyResult) error {
	if targetInfo, err := os.Lstat(target); err == nil {
		if targetInfo.IsDir() {
			if apply {
				if err := ensureOwnerWritable(target, targetInfo.Mode().Perm()); err != nil {
					return err
				}
				result.Skipped++
			} else {
				result.WouldSkip++
			}
			return nil
		}
		return addConflict(rel, result)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !apply {
		result.WouldMake++
		return nil
	}
	if err := os.MkdirAll(target, writableDirMode(info.Mode().Perm())); err != nil {
		return err
	}
	result.MakeDirs++
	return nil
}

func writableDirMode(mode fs.FileMode) fs.FileMode {
	perm := mode.Perm()
	if perm == 0 {
		perm = 0o755
	}
	return perm | 0o700
}

func ensureOwnerWritable(path string, fallback fs.FileMode) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = fallback.Perm()
	}
	if mode&0o200 != 0 {
		return nil
	}
	return os.Chmod(path, writableDirMode(mode))
}

func copyFile(source string, target string, rel string, info fs.FileInfo, apply bool, result *copyResult) error {
	existing, err := os.Lstat(target)
	if err == nil {
		if !existing.Mode().IsRegular() {
			return addConflict(rel, result)
		}
		same, err := sameFileContent(source, target)
		if err != nil {
			return err
		}
		if !same {
			return addConflict(rel, result)
		}
		if apply {
			result.Skipped++
		} else {
			result.WouldSkip++
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !apply {
		result.WouldCopy++
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := ensureOwnerWritable(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := copyRegularFile(source, target, info); err != nil {
		return err
	}
	result.Copied++
	return nil
}

func copySymlink(source string, target string, rel string, apply bool, result *copyResult) error {
	sourceTarget, err := os.Readlink(source)
	if err != nil {
		return err
	}
	if targetTarget, err := os.Readlink(target); err == nil {
		if targetTarget == sourceTarget {
			if apply {
				result.Skipped++
			} else {
				result.WouldSkip++
			}
			return nil
		}
		return addConflict(rel, result)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !apply {
		result.WouldCopy++
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := ensureOwnerWritable(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.Symlink(sourceTarget, target); err != nil {
		return err
	}
	result.Copied++
	return nil
}

func handleSpecial(target string, rel string, result *copyResult) error {
	if _, err := os.Lstat(target); err == nil {
		return addConflict(rel, result)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// FIFOs, devices, sockets and other special files are not copied into the
	// migrated home. Treat them as conflicts so the operator can inspect them.
	return addConflict(rel, result)
}

func addConflict(rel string, result *copyResult) error {
	result.Conflicts++
	result.ConflictPaths = append(result.ConflictPaths, rel)
	return nil
}

func sameFileContent(left string, right string) (bool, error) {
	leftData, err := os.ReadFile(left)
	if err != nil {
		return false, err
	}
	rightData, err := os.ReadFile(right)
	if err != nil {
		return false, err
	}
	return bytes.Equal(leftData, rightData), nil
}

func copyRegularFile(source string, target string, info fs.FileInfo) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(target)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(target)
		return closeErr
	}
	mtime := info.ModTime()
	if mtime.IsZero() {
		mtime = time.Now()
	}
	return os.Chtimes(target, mtime, mtime)
}

func printSummary(w io.Writer, opts options, sum summary) {
	if opts.apply {
		fmt.Fprintf(w, "\nsummary: threads=%d old_homes=%d copied=%d skipped=%d dirs_created=%d conflicts=%d\n", sum.Threads, sum.Homes, sum.Copied, sum.Skipped, sum.MakeDirs, sum.Conflicts)
		return
	}
	fmt.Fprintf(w, "\nsummary: dry_run=true threads=%d old_homes=%d would_copy=%d would_skip=%d would_create_dirs=%d conflicts=%d\n", sum.Threads, sum.Homes, sum.WouldCopy, sum.WouldSkip, sum.WouldMake, sum.Conflicts)
	fmt.Fprintln(w, "run again with --apply to copy files")
}
