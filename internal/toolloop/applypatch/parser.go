package applypatch

import (
	"fmt"
	"strings"
)

func Parse(raw string) (Patch, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	lines := strings.Split(raw, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	p := patchParser{lines: lines}
	return p.parse()
}

type patchParser struct {
	lines []string
	i     int
}

func (p *patchParser) parse() (Patch, error) {
	if !p.consume("*** Begin Patch") {
		return Patch{}, p.err("missing *** Begin Patch")
	}
	var hunks []Hunk
	for !p.done() && p.peek() != "*** End Patch" {
		hunk, err := p.parseHunk()
		if err != nil {
			return Patch{}, err
		}
		hunks = append(hunks, hunk)
	}
	if !p.consume("*** End Patch") {
		return Patch{}, p.err("missing *** End Patch")
	}
	if len(hunks) == 0 {
		return Patch{}, p.err("patch contains no hunks")
	}
	return Patch{Hunks: hunks}, nil
}

func (p *patchParser) parseHunk() (Hunk, error) {
	line := p.peek()
	switch {
	case strings.HasPrefix(line, "*** Add File: "):
		return p.parseAddFile()
	case strings.HasPrefix(line, "*** Delete File: "):
		return p.parseDeleteFile()
	case strings.HasPrefix(line, "*** Update File: "):
		return p.parseUpdateFile()
	default:
		return nil, p.err("expected hunk header")
	}
}

func (p *patchParser) parseAddFile() (AddFile, error) {
	path := strings.TrimSpace(strings.TrimPrefix(p.next(), "*** Add File: "))
	if path == "" {
		return AddFile{}, p.err("missing add file path")
	}
	var b strings.Builder
	for !p.done() && !isPatchBoundary(p.peek()) {
		line := p.next()
		if !strings.HasPrefix(line, "+") {
			return AddFile{}, p.err("add file lines must start with +")
		}
		b.WriteString(strings.TrimPrefix(line, "+"))
		b.WriteString("\n")
	}
	return AddFile{Path: path, Content: b.String()}, nil
}

func (p *patchParser) parseDeleteFile() (DeleteFile, error) {
	path := strings.TrimSpace(strings.TrimPrefix(p.next(), "*** Delete File: "))
	if path == "" {
		return DeleteFile{}, p.err("missing delete file path")
	}
	return DeleteFile{Path: path}, nil
}

func (p *patchParser) parseUpdateFile() (UpdateFile, error) {
	path := strings.TrimSpace(strings.TrimPrefix(p.next(), "*** Update File: "))
	if path == "" {
		return UpdateFile{}, p.err("missing update file path")
	}
	hunk := UpdateFile{Path: path}
	if !p.done() && strings.HasPrefix(p.peek(), "*** Move to: ") {
		hunk.MoveTo = strings.TrimSpace(strings.TrimPrefix(p.next(), "*** Move to: "))
		if hunk.MoveTo == "" {
			return UpdateFile{}, p.err("missing move destination")
		}
	}
	for !p.done() && !isPatchBoundary(p.peek()) {
		chunk, err := p.parseUpdateChunk()
		if err != nil {
			return UpdateFile{}, err
		}
		hunk.Chunks = append(hunk.Chunks, chunk)
	}
	return chunklessMoveOK(hunk)
}

func chunklessMoveOK(hunk UpdateFile) (UpdateFile, error) {
	if len(hunk.Chunks) == 0 && strings.TrimSpace(hunk.MoveTo) == "" {
		return UpdateFile{}, fmt.Errorf("update file %s has no chunks", hunk.Path)
	}
	return hunk, nil
}

func (p *patchParser) parseUpdateChunk() (UpdateChunk, error) {
	if !strings.HasPrefix(p.peek(), "@@") {
		return UpdateChunk{}, p.err("expected @@ update chunk")
	}
	context := strings.TrimSpace(strings.TrimPrefix(p.next(), "@@"))
	chunk := UpdateChunk{Context: context}
	for !p.done() && !strings.HasPrefix(p.peek(), "@@") && !isPatchBoundary(p.peek()) {
		line := p.next()
		if line == "*** End of File" {
			chunk.EndOfFile = true
			continue
		}
		if line == "" {
			return UpdateChunk{}, p.err("update lines must start with space, +, or -")
		}
		text := line[1:] + "\n"
		switch line[0] {
		case ' ':
			chunk.OldLines = append(chunk.OldLines, text)
			chunk.NewLines = append(chunk.NewLines, text)
		case '-':
			chunk.OldLines = append(chunk.OldLines, text)
		case '+':
			chunk.NewLines = append(chunk.NewLines, text)
		default:
			return UpdateChunk{}, p.err("update lines must start with space, +, or -")
		}
	}
	if len(chunk.OldLines) == 0 && len(chunk.NewLines) == 0 {
		return UpdateChunk{}, p.err("empty update chunk")
	}
	return chunk, nil
}

func isPatchBoundary(line string) bool {
	return line == "*** End Patch" || strings.HasPrefix(line, "*** Add File: ") || strings.HasPrefix(line, "*** Delete File: ") || strings.HasPrefix(line, "*** Update File: ")
}

func (p *patchParser) done() bool { return p.i >= len(p.lines) }
func (p *patchParser) peek() string {
	if p.done() {
		return ""
	}
	return p.lines[p.i]
}
func (p *patchParser) next() string {
	line := p.peek()
	p.i++
	return line
}
func (p *patchParser) consume(line string) bool {
	if p.peek() != line {
		return false
	}
	p.i++
	return true
}
func (p *patchParser) err(msg string) error { return fmt.Errorf("line %d: %s", p.i+1, msg) }
