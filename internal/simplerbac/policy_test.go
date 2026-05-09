package simplerbac

import "testing"

type testSubject struct {
	roles []Role
}

func (s testSubject) HasRole(role Role) bool {
	for _, candidate := range s.roles {
		if candidate == role {
			return true
		}
	}
	return false
}

func TestRuleAllowsPublicOrMatchingRole(t *testing.T) {
	if !Public().Allows(testSubject{}) {
		t.Fatalf("public rule should allow empty actor")
	}
	if !Any(RoleRoot).Allows(testSubject{roles: []Role{RoleUser, RoleRoot}}) {
		t.Fatalf("root rule should allow root actor")
	}
	if Any(RoleRoot).Allows(testSubject{roles: []Role{RoleUser}}) {
		t.Fatalf("root rule should deny user actor")
	}
	if !Any(RoleRoot, RoleElevated).Allows(testSubject{roles: []Role{RoleUser, RoleElevated}}) {
		t.Fatalf("root/elevated rule should allow elevated actor")
	}
	if Any(RoleElevated).Allows(testSubject{roles: []Role{RoleUser}}) {
		t.Fatalf("elevated rule should deny plain user actor")
	}
}
