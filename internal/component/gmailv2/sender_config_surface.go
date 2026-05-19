package gmailv2

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
)

const (
	senderConfigExplicitPolicy = "explicit-policy"
	senderConfigTrusted        = "trusted"
	senderConfigShowFull       = "show-full"
	senderConfigNotifyAgent    = "notify-agent"
)

type senderConfigSurface struct {
	component *Component
	email     string
	policy    *senderPolicy
}

func (s *senderConfigSurface) ConfigSchema(ctx context.Context, req commandengine.Request) (configsurface.ConfigSchema, error) {
	_, _ = ctx, req
	showFullDefault := false
	if s != nil && s.component != nil {
		showFullDefault = s.component.componentConfig.DefaultShowFull
	}
	return configsurface.ConfigSchema{Fields: []configsurface.FieldSchema{
		{Key: senderConfigExplicitPolicy, Help: "Whether this sender has an explicit policy row", Type: configsurface.FieldTypeBool, Writable: false, Default: "false", Options: []string{"true", "false"}},
		{Key: senderConfigTrusted, Help: "Treat this sender as trusted", Type: configsurface.FieldTypeBool, Writable: true, Default: "false", Options: []string{"true", "false"}},
		{Key: senderConfigShowFull, Help: "Show full body text from this sender in inbound prompts", Type: configsurface.FieldTypeBool, Writable: true, Default: strconv.FormatBool(showFullDefault), Options: []string{"true", "false"}},
		{Key: senderConfigNotifyAgent, Help: "Notify the agent when this sender sends mail", Type: configsurface.FieldTypeBool, Writable: true, Default: "true", Options: []string{"true", "false"}},
	}}, nil
}

func (s *senderConfigSurface) ConfigGet(ctx context.Context, req commandengine.Request, key string) (string, error) {
	_, _ = ctx, req
	if s == nil || s.component == nil {
		return "", fmt.Errorf("missing gmailv2 sender config")
	}
	policy := s.policy
	switch configsurface.NormalizeKey(key) {
	case senderConfigExplicitPolicy:
		return strconv.FormatBool(policy != nil), nil
	case senderConfigTrusted:
		return strconv.FormatBool(policy != nil && policy.Trusted), nil
	case senderConfigShowFull:
		if policy != nil {
			return strconv.FormatBool(policy.ShowFull), nil
		}
		return strconv.FormatBool(s.component.componentConfig.DefaultShowFull), nil
	case senderConfigNotifyAgent:
		return strconv.FormatBool(policy == nil || !policy.StoreOnly), nil
	default:
		return "", unknownSenderConfig(key)
	}
}

func (s *senderConfigSurface) ConfigSet(ctx context.Context, req commandengine.Request, key string, value string) error {
	_ = req
	if s == nil || s.component == nil {
		return fmt.Errorf("missing gmailv2 sender config")
	}
	key = configsurface.NormalizeKey(key)
	parsed, err := configsurface.ParseBool(value)
	if err != nil {
		return fmt.Errorf("sender config %s expects true or false", key)
	}
	switch key {
	case senderConfigTrusted:
		return s.save(ctx, func(p *senderPolicy) { p.Trusted = parsed })
	case senderConfigShowFull:
		return s.save(ctx, func(p *senderPolicy) { p.ShowFull = parsed })
	case senderConfigNotifyAgent:
		return s.save(ctx, func(p *senderPolicy) { p.StoreOnly = !parsed })
	case senderConfigExplicitPolicy:
		return fmt.Errorf("sender config %s is read-only", key)
	default:
		return unknownSenderConfig(key)
	}
}

func (s *senderConfigSurface) ConfigUnset(ctx context.Context, req commandengine.Request, key string) error {
	_ = req
	if s == nil || s.component == nil {
		return fmt.Errorf("missing gmailv2 sender config")
	}
	key = configsurface.NormalizeKey(key)
	switch key {
	case senderConfigTrusted:
		return s.save(ctx, func(p *senderPolicy) { p.Trusted = false })
	case senderConfigShowFull:
		defaultShowFull := s.component.componentConfig.DefaultShowFull
		return s.save(ctx, func(p *senderPolicy) { p.ShowFull = defaultShowFull })
	case senderConfigNotifyAgent:
		return s.save(ctx, func(p *senderPolicy) { p.StoreOnly = false })
	case senderConfigExplicitPolicy:
		return fmt.Errorf("sender config %s is read-only", key)
	default:
		return unknownSenderConfig(key)
	}
}

func (s *senderConfigSurface) save(ctx context.Context, update func(*senderPolicy)) error {
	if strings.TrimSpace(s.email) == "" {
		return fmt.Errorf("missing sender email")
	}
	return s.component.store.saveSenderPolicy(ctx, s.email, update)
}

func (c *Component) senderConfigSurface(ctx context.Context, email string) (*senderConfigSurface, error) {
	email = normalizeEmail(email)
	if email == "" {
		return nil, fmt.Errorf("missing sender email")
	}
	policy, err := c.store.senderPolicy(ctx, email)
	if err != nil {
		return nil, err
	}
	return &senderConfigSurface{component: c, email: email, policy: policy}, nil
}

func unknownSenderConfig(key string) error {
	return fmt.Errorf("unsupported sender config key %q; use trusted, show-full, or notify-agent", configsurface.NormalizeKey(key))
}
