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
