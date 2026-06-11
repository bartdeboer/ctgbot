package agentcommon

import "strings"

// HostbridgeExampleLines returns compact examples for core command families that
// are present in this thread's hostbridge command synopsis. Keep examples sparse:
// they are agent-facing orientation, not a second help system.
func HostbridgeExampleLines(controlSynopsis string) []string {
	var lines []string
	if strings.Contains(controlSynopsis, "heartbeat [") {
		lines = append(lines,
			"thread heartbeat examples: `hostbridge thread heartbeat 2h`; workday cron: `hostbridge thread heartbeat \"CRON_TZ=Europe/Amsterdam 0 9-17/2 * * 1-5\" \"check theater updates\"`; immediate check: `hostbridge heartbeat now`",
			"thread wake examples: `hostbridge thread wake once 20m \"check if download completed\"`; `hostbridge thread wake schedule \"0 3 * * *\" \"backup database\"`",
			"thread wake cleanup: `hostbridge thread wake list`; `hostbridge thread heartbeat clear`; `hostbridge thread wake once clear`; `hostbridge thread wake schedule clear all`",
		)
	}
	if strings.Contains(controlSynopsis, "theater [") {
		lines = append(lines,
			"theater examples: `hostbridge theater <thread> subscribe`; `hostbridge theater <thread> read`",
		)
	}
	return lines
}
