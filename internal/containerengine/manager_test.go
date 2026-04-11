package containerengine

import "testing"

func TestIsMissingContainerOutputIsCaseInsensitive(t *testing.T) {
	t.Parallel()

	cases := []string{
		"Error response from daemon: No such object: ctgbot-test",
		"error: no such object: ctgbot-test",
		"Error response from daemon: No such container: ctgbot-test",
	}

	for _, msg := range cases {
		msg := msg
		t.Run(msg, func(t *testing.T) {
			t.Parallel()
			if !isMissingContainerOutput(msg) {
				t.Fatalf("expected %q to be treated as a missing container", msg)
			}
		})
	}
}

func TestBuildCreateArgsIncludesGPUs(t *testing.T) {
	t.Parallel()

	args := buildCreateArgs(ContainerSpec{
		Name:  "ctgbot-test",
		Image: "ctgbot:latest",
		GPUs:  "all",
		Cmd:   []string{"tail", "-f", "/dev/null"},
	})

	if len(args) == 0 {
		t.Fatalf("expected docker create args")
	}

	found := false
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--gpus" && args[i+1] == "all" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected --gpus all in args: %#v", args)
	}
}
