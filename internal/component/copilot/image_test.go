package copilot

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
)

func TestExternalImageContextAndExplicitDependencies(t *testing.T) {
	for _, uses := range []*runtimeimage.Target{nil, {Image: "custom-base", Dockerfile: "custom-base.Dockerfile"}} {
		c := &Component{Core: agentcommon.Core{RuntimeImageContext: "/host/images", RuntimeImage: "custom", RuntimeDockerfile: "recipe.Dockerfile", RuntimeImageUses: uses}}
		targets, err := c.RuntimeImageTargets(t.Context())
		if err != nil || len(targets) != 1 || targets[0].Uses != uses || targets[0].Context != "/host/images" || targets[0].Dockerfile != "recipe.Dockerfile" {
			t.Fatal(targets, err)
		}
	}
}
