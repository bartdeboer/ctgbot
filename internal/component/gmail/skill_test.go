package gmail

import (
	"strings"
	"testing"
)

func TestSkillContainsSetupCommands(t *testing.T) {
	text := (&Component{}).Skill().Text
	for _, want := range []string{
		"hostbridge component gmail/work managed-file put oauth_client.json",
		"ctgbot component gmail/work auth",
		"hostbridge component gmail/work auth status",
		"ctgbot chat <chatID> component add source gmail/work",
		"hostbridge component gmail/work message",
		"mailbox_email",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Skill text missing %q:\n%s", want, text)
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "hostbridge component gmail/work auth" {
			t.Fatalf("Skill text suggests blocking hostbridge auth:\n%s", text)
		}
	}
}
