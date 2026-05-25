package agentcommon

import "github.com/bartdeboer/ctgbot/internal/containerengine"

// WrapWithPIDFile records the long-lived command PID inside the runtime
// container so runtime interrupt can send SIGINT to the active agent process.
func WrapWithPIDFile(args []string) []string {
	wrapped := []string{"sh", "-lc", "rm -f " + containerengine.ActivePIDFile + "; echo $$ > " + containerengine.ActivePIDFile + "; exec \"$@\"", "sh"}
	return append(wrapped, args...)
}
