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
			name:  "no semver tag uses base with commit",
			state: gitVersionState{ShortCommit: "abc123"},
			want:  "v0.0.0-abc123",
		},
		{
			name:  "dirty no semver tag uses base with commit",
			state: gitVersionState{ShortCommit: "abc123", Dirty: true},
			want:  "v0.0.0-abc123-dev",
		},
		{
			name:  "no git state falls back to dev",
			state: gitVersionState{},
			want:  "v0.0.0-dev",
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

func TestVersionInfoFromEmbeddedText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body []byte
		want VersionInfo
	}{
		{name: "missing generated version", want: VersionInfo{Version: "v0.0.0-dev"}},
		{name: "blank generated version", body: []byte("\n"), want: VersionInfo{Version: "v0.0.0-dev"}},
		{name: "generated version", body: []byte(" v1.2.3-abc123\n"), want: VersionInfo{Version: "v1.2.3-abc123", Generated: true}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := versionInfoFromEmbeddedText(tc.body, nil); got != tc.want {
				t.Fatalf("versionInfoFromEmbeddedText() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestVersionReturnsInfoVersion(t *testing.T) {
	t.Parallel()

	if got, want := Version(), Info().Version; got != want {
		t.Fatalf("Version() = %q, want %q", got, want)
	}
}
