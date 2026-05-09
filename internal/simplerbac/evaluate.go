package simplerbac

import "fmt"

type Subject interface {
	HasRole(role Role) bool
}

func (r Rule) Allows(actor Subject) bool {
	if len(r.AnyRole) == 0 {
		return true
	}
	if actor == nil {
		return false
	}
	for _, role := range r.AnyRole {
		if actor.HasRole(role) {
			return true
		}
	}
	return false
}

func (r Rule) Check(actor Subject) error {
	if r.Allows(actor) {
		return nil
	}
	return fmt.Errorf("requires one of roles: %v", r.AnyRole)
}
