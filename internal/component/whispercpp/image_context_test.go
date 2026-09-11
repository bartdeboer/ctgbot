package whispercpp

import (
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
	"testing"
)

func TestExternalImageContextWiring(t *testing.T) {
	for _, dep := range []*runtimeimage.Target{nil, {Image: "base", Dockerfile: "base.Dockerfile"}} {
		c := &Component{runtimeConfig: runtimepkg.BindConfig{Context: "/host/images", Uses: dep}}
		targets, err := c.RuntimeImageTargets(t.Context())
		if err != nil || len(targets) != 1 {
			t.Fatalf("targets=%v err=%v", targets, err)
		}
		if targets[0].Context != "/host/images" || targets[0].Uses != dep {
			t.Fatalf("target=%+v", targets[0])
		}
	}
}
