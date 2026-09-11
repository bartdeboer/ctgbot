package claude

import (
	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
	"testing"
)

func TestExternalImageContextWiring(t *testing.T) {
	for _, dep := range []*runtimeimage.Target{nil, {Image: "base", Dockerfile: "base.Dockerfile"}} {
		c := &Component{Core: agentcommon.Core{RuntimeImageContext: "/host/images", RuntimeImageUses: dep}}
		targets, err := c.RuntimeImageTargets(t.Context())
		if err != nil || len(targets) != 1 {
			t.Fatalf("targets=%v err=%v", targets, err)
		}
		if targets[0].Context != "/host/images" || targets[0].Uses != dep {
			t.Fatalf("target=%+v", targets[0])
		}
	}
}
