package buildassets

import "testing"

func TestDeriveVersionFromGitState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state gitVersionState
		want  string
	}{
		{
			name:  "clean exact semver tag",
			state: gitVersionState{ExactTag: "v1.2.3", NearestTag: "v1.2.3", ShortCommit: "abc123"},
			want:  "v1.2.3",
		},
		{
			name:  "clean nearest semver tag",
			state: gitVersionState{NearestTag: "v1.2.3", ShortCommit: "abc123"},
			want:  "v1.2.3-abc123",
		},
		{
			name:  "dirty nearest semver tag",
			state: gitVersionState{NearestTag: "v1.2.3", ShortCommit: "abc123", Dirty: true},
			want:  "v1.2.3-abc123-dev",
		},
		{
			name:  "dirty exact semver tag",
			state: gitVersionState{ExactTag: "v1.2.3", NearestTag: "v1.2.3", ShortCommit: "abc123", Dirty: true},
			want:  "v1.2.3-abc123-dev",
		},
		{
			name:  "no semver tag uses base",
			state: gitVersionState{ShortCommit: "abc123"},
			want:  "v0.1.0-abc123",
		},
		{
			name:  "no git state falls back to dev",
			state: gitVersionState{},
			want:  "v0.1.0-dev",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := deriveVersionFromGitState(tc.state); got != tc.want {
				t.Fatalf("deriveVersionFromGitState() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVersionFallsBackWhenGeneratedFileMissing(t *testing.T) {
	t.Parallel()

	if got := Version(); got == "" {
		t.Fatalf("Version() returned empty string")
	}
}
