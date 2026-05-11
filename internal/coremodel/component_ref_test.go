package coremodel

import "testing"

func TestParseComponentRef(t *testing.T) {
	tests := []struct {
		name         string
		ref          string
		wantType     string
		wantName     string
		wantExplicit bool
		wantErr      bool
	}{
		{name: "default", ref: "gmail", wantType: "gmail", wantName: "gmail"},
		{name: "named", ref: "gmail/work", wantType: "gmail", wantName: "work", wantExplicit: true},
		{name: "trimmed", ref: "  telegram / bot2  ", wantType: "telegram", wantName: "bot2", wantExplicit: true},
		{name: "missing", ref: "", wantErr: true},
		{name: "missing name", ref: "gmail/", wantErr: true},
		{name: "too deep", ref: "a/b/c", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseComponentRef(tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseComponentRef(%q) error = nil, want error", tt.ref)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseComponentRef(%q) error = %v", tt.ref, err)
			}
			if got.Type != tt.wantType || got.Name != tt.wantName || got.ExplicitName != tt.wantExplicit {
				t.Fatalf("ParseComponentRef(%q) = %#v", tt.ref, got)
			}
		})
	}
}

func TestChatComponentRoleValid(t *testing.T) {
	if !ChatComponentRoleSource.Valid() || !ChatComponentRoleRelay.Valid() || !ChatComponentRoleAgent.Valid() || !ChatComponentRoleCommand.Valid() {
		t.Fatal("expected known roles to be valid")
	}
	if ChatComponentRole("weird").Valid() {
		t.Fatal("expected unknown role to be invalid")
	}
}

func TestComponentBindingRoleValid(t *testing.T) {
	if !ComponentBindingRoleGuard.Valid() {
		t.Fatal("expected guard binding role to be valid")
	}
	if ComponentBindingRole("weird").Valid() {
		t.Fatal("expected unknown binding role to be invalid")
	}
}
