package agentcommon

import "strings"

// HostbridgeExampleLines returns compact examples for core command families that
// are present in this thread's hostbridge command synopsis. Keep examples sparse:
// they are agent-facing orientation, not a second help system.
func HostbridgeExampleLines(controlSynopsis string) []string {
	var lines []string
	if strings.Contains(controlSynopsis, "heartbeat [") {
		lines = append(lines,
			"heartbeat examples: `hostbridge heartbeat start 2h`; workday cron: `hostbridge heartbeat start cron \"CRON_TZ=Europe/Amsterdam 0 9-17/2 * * 1-5\" --reason \"check theater updates\"`; immediate check: `hostbridge heartbeat now`",
		)
	}
	if strings.Contains(controlSynopsis, "theater [") {
		lines = append(lines,
			"theater examples: `hostbridge theater <thread> subscribe`; `hostbridge theater <thread> read`",
		)
	}
	return lines
}
