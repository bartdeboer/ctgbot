package applypatch

import "testing"

func TestParseAddDeleteUpdate(t *testing.T) {
	t.Parallel()
	patch, err := Parse(`*** Begin Patch
*** Add File: hello.txt
+hello
*** Delete File: old.txt
*** Update File: readme.md
@@
-old
+new
*** End Patch`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(patch.Hunks) != 3 {
		t.Fatalf("len(Hunks) = %d", len(patch.Hunks))
	}
	if _, ok := patch.Hunks[0].(AddFile); !ok {
		t.Fatalf("hunk[0] = %T", patch.Hunks[0])
	}
}

func TestParseRejectsMissingBoundary(t *testing.T) {
	t.Parallel()
	_, err := Parse(`*** Add File: hello.txt
+hello`)
	if err == nil {
		t.Fatalf("Parse() error = nil, want error")
	}
}
