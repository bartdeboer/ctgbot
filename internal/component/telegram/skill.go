package telegram

import "github.com/bartdeboer/ctgbot/internal/component"

var _ component.SkillProvider = (*Component)(nil)

func (c *Component) Skill() component.Skill {
	return component.Skill{
		Name:        Type,
		Description: "Telegram bot source and relay setup",
		Text: `Telegram component

What it does:
- Receives Telegram bot updates as inbound source events.
- Sends outbound messages back to Telegram as a relay.
- Stores the bot token and Telegram settings in this component's profile home.

Managed files:
- token.txt: Telegram bot token. Sensitive. Required before polling or sending works.
- component.json: optional Telegram settings file.

Optional component.json settings:
{
  "operators": [123456789],
  "poll_timeout": "60s",
  "debounce_window": "800ms",
  "render_format": "plain"
}

Setup commands:
1. Register the component:
   ctgbot component register telegram/telegram --runtime local

2. Install the bot token safely:
   printf '%s' "$TELEGRAM_BOT_TOKEN" | hostbridge component telegram/telegram managed-file put token.txt

3. Optionally install component.json:
   printf '{"operators":[123456789],"render_format":"plain"}' | hostbridge component telegram/telegram managed-file put component.json --type application/json

4. Check managed files:
   hostbridge component telegram/telegram managed-file list
   hostbridge component telegram/telegram managed-file status

Safety notes:
- Never paste token.txt contents into chat.
- managed-file status only reports present/missing; it does not print sensitive contents.
- operators in component.json get root command privileges for Telegram messages from those user ids.`,
	}
}
