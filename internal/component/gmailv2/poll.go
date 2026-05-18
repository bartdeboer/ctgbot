package gmailv2

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/component"
	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
)

func (c *Component) RunInbound(ctx context.Context, emit component.InboundEmitter) error {
	if emit == nil {
		return fmt.Errorf("missing inbound emitter")
	}
	interval := c.componentConfig.pollInterval()
	client, err := c.waitForClient(ctx, interval)
	if err != nil {
		return err
	}
	var state mailboxState
	for {
		if state.HistoryID == 0 {
			baseline, err := c.ensureStateBaseline(ctx, client)
			if err != nil {
				if waitErr := c.waitAfterPollError(ctx, interval, "baseline", err); waitErr != nil {
					return waitErr
				}
				continue
			}
			state = baseline
		}
		if err := c.pollOnce(ctx, client, &state, emit); err != nil {
			if waitErr := c.waitAfterPollError(ctx, interval, "poll", err); waitErr != nil {
				return waitErr
			}
			continue
		}
		if err := waitInterval(ctx, interval); err != nil {
			return err
		}
	}
}

func (c *Component) waitAfterPollError(ctx context.Context, interval time.Duration, phase string, err error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	c.logf("gmailv2 %s failed component=%s err=%v", phase, c.registration.Ref(), err)
	return waitInterval(ctx, interval)
}

func (c *Component) waitForClient(ctx context.Context, interval time.Duration) (gmailClient, error) {
	logged := false
	for {
		client, err := c.client(ctx)
		if err == nil {
			if logged {
				c.logf("gmailv2 source authenticated component=%s", c.registration.Ref())
			}
			return client, nil
		}
		if !isMissingAuthMaterial(err) {
			return nil, err
		}
		if !logged {
			c.logf("gmailv2 source unavailable component=%s err=%v", c.registration.Ref(), err)
			logged = true
		}
		if waitErr := waitInterval(ctx, interval); waitErr != nil {
			return nil, waitErr
		}
	}
}

func waitInterval(ctx context.Context, interval time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Component) ensureStateBaseline(ctx context.Context, client gmailClient) (mailboxState, error) {
	profile, err := client.GetProfile(ctx, c.userID())
	if err != nil {
		return mailboxState{}, fmt.Errorf("get gmailv2 profile: %w", err)
	}
	state, err := c.loadState()
	if err != nil {
		return mailboxState{}, err
	}
	if email := strings.TrimSpace(profile.EmailAddress); email != "" {
		c.mailboxEmail = email
		state.MailboxEmail = email
	}
	if state.HistoryID == 0 {
		state.HistoryID = profile.HistoryId
		state.LastSyncedAt = time.Now().UTC()
		if err := c.saveState(state); err != nil {
			return mailboxState{}, err
		}
	}
	return state, nil
}

func (c *Component) pollOnce(ctx context.Context, client gmailClient, state *mailboxState, emit component.InboundEmitter) error {
	if state == nil {
		return fmt.Errorf("missing gmailv2 state")
	}
	if state.HistoryID == 0 {
		baseline, err := c.ensureStateBaseline(ctx, client)
		if err != nil {
			return err
		}
		*state = baseline
		return nil
	}
	pageToken := ""
	seenMessages := map[string]struct{}{}
	currentHistoryID := state.HistoryID
	emitted := 0
	hitLimit := false
	for {
		if emitted >= c.componentConfig.MaxPollMessages {
			hitLimit = true
			break
		}
		response, err := client.ListHistory(ctx, c.userID(), currentHistoryID, pageToken)
		if err != nil {
			if isHistoryExpired(err) {
				baseline, baselineErr := c.resetHistoryBaseline(ctx, client)
				if baselineErr != nil {
					return baselineErr
				}
				*state = baseline
				return nil
			}
			return fmt.Errorf("list gmailv2 history: %w", err)
		}
		for _, history := range response.History {
			for _, added := range history.MessagesAdded {
				if emitted >= c.componentConfig.MaxPollMessages {
					hitLimit = true
					break
				}
				if added == nil || added.Message == nil {
					continue
				}
				messageID := strings.TrimSpace(added.Message.Id)
				if messageID == "" {
					continue
				}
				if _, seen := seenMessages[messageID]; seen {
					continue
				}
				seenMessages[messageID] = struct{}{}
				message, err := client.GetMessage(ctx, c.userID(), messageID)
				if err != nil {
					if isGmailNotFound(err) {
						c.logf("gmailv2 message no longer available component=%s message=%s", c.registration.Ref(), messageID)
						continue
					}
					return fmt.Errorf("get gmailv2 message %s: %w", messageID, err)
				}
				if c.shouldSkipMessage(message) {
					continue
				}
				incoming, fresh, err := c.storeIncomingMessage(ctx, client, message)
				if err != nil {
					return err
				}
				if !fresh {
					continue
				}
				storeOnly, err := c.senderStoreOnly(ctx, incoming.Record.FromEmail)
				if err != nil {
					return err
				}
				if storeOnly {
					state.LastMessageID = strings.TrimSpace(message.Id)
					continue
				}
				if err := emit(ctx, c.inboundEventFromStoredMessage(incoming.Record, incoming.TextBody)); err != nil {
					return err
				}
				emitted++
				state.LastMessageID = strings.TrimSpace(message.Id)
			}
		}
		if hitLimit {
			break
		}
		if response.HistoryId != 0 {
			state.HistoryID = response.HistoryId
		}
		if emitted >= c.componentConfig.MaxPollMessages {
			hitLimit = true
			break
		}
		pageToken = strings.TrimSpace(response.NextPageToken)
		if pageToken == "" {
			break
		}
	}
	state.LastSyncedAt = time.Now().UTC()
	return c.saveState(*state)
}

func (c *Component) resetHistoryBaseline(ctx context.Context, client gmailClient) (mailboxState, error) {
	profile, err := client.GetProfile(ctx, c.userID())
	if err != nil {
		return mailboxState{}, fmt.Errorf("reset gmailv2 history baseline: %w", err)
	}
	state := mailboxState{
		MailboxEmail: strings.TrimSpace(profile.EmailAddress),
		HistoryID:    profile.HistoryId,
		LastSyncedAt: time.Now().UTC(),
	}
	c.mailboxEmail = state.MailboxEmail
	if err := c.saveState(state); err != nil {
		return mailboxState{}, err
	}
	return state, nil
}

func (c *Component) shouldSkipMessage(message *gmailapi.Message) bool {
	if message == nil {
		return true
	}
	if len(c.componentConfig.SkipLabels) == 0 {
		return false
	}
	labels := map[string]struct{}{}
	for _, label := range message.LabelIds {
		labels[strings.ToUpper(strings.TrimSpace(label))] = struct{}{}
	}
	for _, label := range c.componentConfig.SkipLabels {
		if _, ok := labels[strings.ToUpper(strings.TrimSpace(label))]; ok {
			return true
		}
	}
	return false
}

func (c *Component) senderStoreOnly(ctx context.Context, email string) (bool, error) {
	policy, err := c.store.senderPolicy(ctx, email)
	if err != nil || policy == nil {
		return false, err
	}
	return policy.StoreOnly, nil
}

func isHistoryExpired(err error) bool {
	return isGmailNotFound(err)
}

func isGmailNotFound(err error) bool {
	var apiErr *googleapi.Error
	return errors.As(err, &apiErr) && apiErr.Code == 404
}
