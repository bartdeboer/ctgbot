package telegram

import (
	"strings"
	"testing"
)

func TestSkillContainsProfileSetupCommands(t *testing.T) {
	text := (&Component{}).Skill().Text

	for _, want := range []string{
		"token.txt",
		"component.json",
		"operators",
		"poll_timeout",
		"debounce_window",
		"render_format",
		"hostbridge component telegram/telegram managed-file put token.txt",
		"hostbridge component telegram/telegram managed-file list",
		"hostbridge component telegram/telegram managed-file status",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Skill text missing %q\n%s", want, text)
		}
	}
}
