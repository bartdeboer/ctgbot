package simplerbac

import "fmt"

type Actor struct {
	Roles []Role
}

func (a Actor) Has(role Role) bool {
	if role == "" {
		return false
	}
	for _, candidate := range a.Roles {
		if candidate == role {
			return true
		}
	}
	return false
}

func (r Rule) Allows(actor Actor) bool {
	if len(r.AnyRole) == 0 {
		return true
	}
	for _, role := range r.AnyRole {
		if actor.Has(role) {
			return true
		}
	}
	return false
}

func (r Rule) Check(actor Actor) error {
	if r.Allows(actor) {
		return nil
	}
	return fmt.Errorf("requires one of roles: %v", r.AnyRole)
}
