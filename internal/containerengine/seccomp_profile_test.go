package containerengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSeccompProfileValidation(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "Browser Policy.JSON")
	if err := os.WriteFile(good, []byte(`{"defaultAction":"SCMP_ACT_ERRNO","syscalls":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	opts, err := SeccompProfileSecurityOpts(good)
	if err != nil || len(opts) != 1 || opts[0] != "seccomp="+good {
		t.Fatalf("%v %v", opts, err)
	}
	for name, contents := range map[string]string{
		"broken": "{", "null": "null", "array": "[]", "empty": "{}", "action": `{"defaultAction":5}`,
		"trailing": `{"defaultAction":"SCMP_ACT_ALLOW"}{}`,
		"syscalls": `{"defaultAction":"SCMP_ACT_ERRNO","syscalls":{}}`,
		"large":    strings.Repeat(" ", (1<<20)+1),
	} {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(root, name)
			if err := os.WriteFile(p, []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := SeccompProfileSecurityOpts(p); err == nil {
				t.Fatal("accepted invalid profile")
			}
		})
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(good, link); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"", "relative.json", root, filepath.Join(root, "missing"), link} {
		if _, err := SeccompProfileSecurityOpts(p); err == nil {
			t.Fatalf("accepted %q", p)
		}
	}
}
