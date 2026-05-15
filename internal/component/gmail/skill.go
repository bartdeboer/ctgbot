package gmail

import "github.com/bartdeboer/ctgbot/internal/component"

func (c *Component) Skill() component.Skill {
	return component.Skill{
		Name:        Type,
		Description: "Gmail inbound source setup and operations",
		Text: `Gmail component

What it does:
- Watches a Gmail mailbox for new mail.
- Emits new messages into ctgbot as normal inbound source events.
- Stores OAuth token and polling state in this component's profile home.

Managed files:
- oauth_client.json: Google OAuth Desktop client JSON. Sensitive. Provide this before auth unless your deployment has a global OAuth client config.
- component.json: optional mailbox/settings file. Recommended before binding sources.
- token.json: written by ctgbot after OAuth. Do not create or print this manually.
- state.json: polling state written by ctgbot.

Recommended component.json:
{
  "mailbox_email": "you@example.com"
}

Setup commands:
1. Register the mailbox component:
   ctgbot component register gmail/work

2. Install the OAuth client config safely:
   cat oauth_client.json | hostbridge component gmail/work managed-file put oauth_client.json --type application/json

3. Optionally install component.json before auth/binding:
   cat component.json | hostbridge component gmail/work managed-file put component.json --type application/json

4. Ask the human/operator to start OAuth from the host CLI:
   ctgbot component gmail/work auth

5. After OAuth, check auth and managed files from hostbridge:
   hostbridge component gmail/work auth status
   hostbridge component gmail/work managed-file status

6. Bind Gmail as a source for a chat:
   ctgbot chat <chatID> component add source gmail/work

Replying:
- Incoming Gmail prompts include Gmail message/thread ids and a ready-to-edit reply command.
- Send a plain-text email/reply:
   hostbridge component gmail/work message '<your reply text>' --to you@example.com --subject 'Re: Subject' --thread-id <gmailThreadId> --in-reply-to <rfcMessageId>

Safety notes:
- Never paste OAuth client secrets or token.json into chat.
- OAuth auth is intentionally a host CLI action; the blocking callback flow is not started through hostbridge.
- managed-file status only reports present/missing; it does not print sensitive contents.
- Keep token.json owned by ctgbot; re-run auth if the token needs replacement.`,
	}
}
