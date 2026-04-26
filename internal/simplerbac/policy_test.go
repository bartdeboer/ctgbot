package simplerbac

import "testing"

func TestRuleAllowsPublicOrMatchingRole(t *testing.T) {
	if !Public().Allows(Actor{}) {
		t.Fatalf("public rule should allow empty actor")
	}
	if !Any(RoleRoot).Allows(Actor{Roles: []Role{RoleUser, RoleRoot}}) {
		t.Fatalf("root rule should allow root actor")
	}
	if Any(RoleRoot).Allows(Actor{Roles: []Role{RoleUser}}) {
		t.Fatalf("root rule should deny user actor")
	}
	if !Any(RoleRoot, RoleElevated).Allows(Actor{Roles: []Role{RoleUser, RoleElevated}}) {
		t.Fatalf("root/elevated rule should allow elevated actor")
	}
	if Any(RoleElevated).Allows(Actor{Roles: []Role{RoleUser}}) {
		t.Fatalf("elevated rule should deny plain user actor")
	}
}
