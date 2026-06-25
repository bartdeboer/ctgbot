package sandboxengine

import "context"

// StartContainers ensures each sandbox container exists and is running.
func StartContainers(ctx context.Context, sandboxes []*Sandbox) error {
	for _, sandbox := range sandboxes {
		if sandbox == nil {
			continue
		}
		if _, err := sandbox.Ensure(ctx); err != nil {
			return err
		}
	}
	return nil
}

// StopContainers stops each sandbox container without deleting it.
func StopContainers(ctx context.Context, sandboxes []*Sandbox) error {
	for _, sandbox := range sandboxes {
		if sandbox == nil {
			continue
		}
		if err := sandbox.Stop(ctx); err != nil {
			return err
		}
	}
	return nil
}

// DeleteContainers removes each sandbox container. Durable mounts are not deleted.
func DeleteContainers(ctx context.Context, sandboxes []*Sandbox) error {
	for _, sandbox := range sandboxes {
		if sandbox == nil {
			continue
		}
		if err := sandbox.Remove(ctx); err != nil {
			return err
		}
	}
	return nil
}
