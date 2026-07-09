package outlook

import "github.com/bartdeboer/ctgbot/internal/component"

func (c *Component) Skill() component.Skill {
	return component.Skill{Name: Type, Description: "Outlook / Office 365 mail source setup and operations", Text: `Outlook component skeleton

What works in this first slice:
- OAuth authentication against Microsoft Graph.
- Auth/status check with /me.
- Poll/list recent Inbox messages.
- Fetch/store/view individual messages in the local outlook.db.
- Emit newly observed messages as ctgbot inbound source events after the initial baseline.

Managed files:
- oauth_client.json: Microsoft app registration JSON, e.g. {"client_id":"...","tenant":"organizations"}. Sensitive.
- token.json: written by ctgbot after OAuth. Sensitive.
- component.json: optional mailbox/settings file.
- state.json: polling state.

Setup:
1. ctgbot component register outlook/teamrockstars
2. cat oauth_client.json | hostbridge component outlook/teamrockstars managed-file put oauth_client.json --type application/json
3. ctgbot component outlook/teamrockstars auth
4. hostbridge component outlook/teamrockstars auth status
5. hostbridge outlook/teamrockstars query recent
6. hostbridge outlook/teamrockstars fetch <outlookMessageId>
7. ctgbot chat <chatID> component add source outlook/teamrockstars

Not implemented yet:
- send
- reply/reply-all
- attachment download/materialization
- delta query; polling currently lists recent folder messages and dedupes locally

Safety:
- Never paste oauth_client.json or token.json into chat.
- OAuth auth is intentionally a host CLI action, not a hostbridge action.`}
}
