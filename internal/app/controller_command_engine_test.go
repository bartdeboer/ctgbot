package app

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestControllerPolicyAddsControllerRoleWithoutGrantingRoot(t *testing.T) {
	policy := controllerPolicy(simplerbac.Any(simplerbac.RoleRoot))
	if !policy.Allows(testRBACSubject{roles: []simplerbac.Role{simplerbac.RoleController}}) {
		t.Fatalf("controller policy does not allow RoleController: %#v", policy.AnyRole)
	}
	if !policy.Allows(testRBACSubject{roles: []simplerbac.Role{simplerbac.RoleRoot}}) {
		t.Fatalf("controller policy stopped allowing RoleRoot: %#v", policy.AnyRole)
	}
}

type testRBACSubject struct{ roles []simplerbac.Role }

func (s testRBACSubject) HasRole(role simplerbac.Role) bool {
	for _, candidate := range s.roles {
		if candidate == role {
			return true
		}
	}
	return false
}
