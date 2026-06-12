package gmailv2

import "github.com/bartdeboer/ctgbot/internal/component"

func (c *Component) Skill() component.Skill {
	return component.Skill{
		Name:        Type,
		Description: "Gmail v2 inbound source setup and operations",
		Text: `Gmail v2 component

What it does:
- Watches a Gmail mailbox for new mail.
- Emits new messages into ctgbot as normal inbound source events.
- Stores OAuth token and polling state in this component's profile.

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
   ctgbot component register gmailv2/work

2. Install the OAuth client config safely:
   cat oauth_client.json | hostbridge component gmailv2/work managed-file put oauth_client.json --type application/json

3. Optionally install component.json before auth/binding:
   cat component.json | hostbridge component gmailv2/work managed-file put component.json --type application/json

4. Ask the human/operator to start OAuth from the host CLI:
   ctgbot component gmailv2/work auth

5. After OAuth, check auth and managed files from hostbridge:
   hostbridge component gmailv2/work auth status
   hostbridge component gmailv2/work managed-file status

6. Bind Gmail as a source for a chat:
   ctgbot chat <chatID> component add source gmailv2/work

Replying:
- Incoming Gmail prompts include Gmail message/thread ids and a ready-to-edit reply command.
- Send a plain-text email/reply:
   hostbridge gmailv2/work message '<your reply text>' --to you@example.com --subject 'Re: Subject' --thread-id <gmailThreadId> --in-reply-to <rfcMessageId>
- Send HTML with an inline cid image:
   hostbridge gmailv2/work message '<h1>Hello</h1><img src="cid:logo">' --type text/html --to you@example.com --subject 'Inline image' --attach '/workspace/out/logo.png;type=image/png;name=logo.png;cid=logo;disposition=inline'

Inbox:
- Search Gmail remotely:
   hostbridge gmailv2/work query 'from:facebook newer_than:7d'
- Fetch and store a Gmail message:
   hostbridge gmailv2/work fetch <gmailMessageId>
- Query the local read-only mailbox store:
   hostbridge gmailv2/work db help
   hostbridge gmailv2/work db schema
   hostbridge gmailv2/work db query 'select id, from_email, subject from messages'
- View stored message metadata:
   hostbridge gmailv2/work message view <storedMessageId>
- Explicitly include the stored body in the command result:
   hostbridge gmailv2/work message view <storedMessageId> --full-body
- Display the stored body to the current chat without returning it in the command result:
   hostbridge gmailv2/work message display <storedMessageId>
- Manage sender policy:
   hostbridge gmailv2/work sender trust sender@example.com
   hostbridge gmailv2/work sender untrust sender@example.com
   hostbridge gmailv2/work sender show-full sender@example.com
   hostbridge gmailv2/work sender hide-full sender@example.com
   hostbridge gmailv2/work sender list
   hostbridge gmailv2/work sender remove sender@example.com

Safety notes:
- Never paste OAuth client secrets or token.json into chat.
- OAuth auth is intentionally a host CLI action; the blocking callback flow is not started through hostbridge.
- managed-file status only reports present/missing; it does not print sensitive contents.
- Keep token.json owned by ctgbot; re-run auth if the token needs replacement.`,
	}
}
