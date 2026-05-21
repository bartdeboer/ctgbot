package applypatch

import "context"

type Request struct {
	Workspace string
	Patch     string
}

type Result struct {
	Summary string
	Delta   Delta
}

type Patch struct {
	Hunks []Hunk
}

type Hunk interface{ hunk() }

type AddFile struct {
	Path    string
	Content string
}

func (AddFile) hunk() {}

type DeleteFile struct{ Path string }

func (DeleteFile) hunk() {}

type UpdateFile struct {
	Path   string
	MoveTo string
	Chunks []UpdateChunk
}

func (UpdateFile) hunk() {}

type UpdateChunk struct {
	Context   string
	OldLines  []string
	NewLines  []string
	EndOfFile bool
}

type Delta struct {
	Changes []Change
	Exact   bool
}

type Change struct {
	Kind       string
	Path       string
	MoveTo     string
	OldContent string
	NewContent string
}

func Apply(ctx context.Context, req Request) (Result, error) {
	_ = ctx
	patch, err := Parse(req.Patch)
	if err != nil {
		return Result{}, err
	}
	return apply(req.Workspace, patch)
}
