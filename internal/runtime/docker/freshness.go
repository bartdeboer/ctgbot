package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/buildassets"
	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

const (
	imageStaleNotice     = "[Runtime notice] The runtime image for this component is older than the installed ctgbot/hostbridge version. Ask the operator to run: /upgrade"
	imageUnstampedNotice = "[Runtime notice] The runtime image for this component has no ctgbot freshness metadata. Some hostbridge commands may be missing or stale. Ask the operator to run: /upgrade"
)

type dockerImageInfo struct {
	ID     string
	Labels map[string]string
}

type dockerContainerInfo struct {
	State   sandboxengine.State
	ImageID string
	Labels  map[string]string
}

func runtimeFreshnessNotices(container dockerContainerInfo, image dockerImageInfo, currentVersion string, currentGitCommit string, componentType string) []string {
	var notices []string
	if container.State != sandboxengine.StateMissing && strings.TrimSpace(container.ImageID) != "" && strings.TrimSpace(image.ID) != "" && container.ImageID != image.ID {
		notices = append(notices, containerStaleNotice(componentType))
	}

	currentVersion = strings.TrimSpace(currentVersion)
	if currentVersion != "" && currentVersion != buildassets.FallbackVersion && strings.TrimSpace(image.ID) != "" {
		imageVersion := strings.TrimSpace(image.Labels[runtimeimage.LabelVersion])
		switch {
		case imageVersion == "":
			notices = append(notices, imageUnstampedNotice)
		case imageVersion != currentVersion:
			notices = append(notices, imageStaleNotice)
		}
		return notices
	}

	currentGitCommit = strings.TrimSpace(currentGitCommit)
	if currentGitCommit == "" || strings.TrimSpace(image.ID) == "" {
		return notices
	}
	imageGitCommit := strings.TrimSpace(image.Labels[runtimeimage.LabelGitCommit])
	switch {
	case imageGitCommit == "":
		notices = append(notices, imageUnstampedNotice)
	case imageGitCommit != currentGitCommit:
		notices = append(notices, imageStaleNotice)
	}
	return notices
}

func containerStaleNotice(componentType string) string {
	componentType = strings.Trim(strings.TrimSpace(componentType), "/")
	if componentType == "" {
		return "[Runtime notice] Your runtime container was created from an older image. Some hostbridge commands may be missing or stale. Ask the operator to refresh this component's runtime container."
	}
	return fmt.Sprintf("[Runtime notice] Your runtime container was created from an older image. Some hostbridge commands may be missing or stale. Ask the operator to run: /%s container refresh", componentType)
}

func inspectDockerImage(ctx context.Context, image string) (dockerImageInfo, error) {
	out, err := dockerInspectJSON(ctx, "image", "inspect", "--format", "{{json .}}", image)
	if err != nil {
		return dockerImageInfo{}, err
	}
	var raw struct {
		ID     string `json:"Id"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return dockerImageInfo{}, err
	}
	return dockerImageInfo{ID: strings.TrimSpace(raw.ID), Labels: raw.Config.Labels}, nil
}

func inspectDockerContainer(ctx context.Context, name string) (dockerContainerInfo, error) {
	out, err := dockerInspectJSON(ctx, "inspect", "--format", "{{json .}}", name)
	if err != nil {
		if isMissingDockerObjectOutput(string(out)) {
			return dockerContainerInfo{State: sandboxengine.StateMissing}, nil
		}
		return dockerContainerInfo{}, err
	}
	var raw struct {
		Image  string `json:"Image"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		State struct {
			Status string `json:"Status"`
		} `json:"State"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return dockerContainerInfo{}, err
	}
	return dockerContainerInfo{
		State:   sandboxengine.State(strings.TrimSpace(raw.State.Status)),
		ImageID: strings.TrimSpace(raw.Image),
		Labels:  raw.Config.Labels,
	}, nil
}

func dockerInspectJSON(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	return cmd.CombinedOutput()
}

func isMissingDockerObjectOutput(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	return strings.Contains(lower, "no such object") || strings.Contains(lower, "no such container")
}
