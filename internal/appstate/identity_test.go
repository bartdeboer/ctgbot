package appstate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveGitIdentityPrefersExplicitConfig(t *testing.T) {
	cfg, store := newTestConfig(t)
	t.Setenv("GIT_CONFIG_GLOBAL", writeGitConfig(t, "Host User", "host@example.com"))
	if err := store.PersistString("git.user_name", "Config User"); err != nil {
		t.Fatalf("PersistString(user_name) error = %v", err)
	}
	if err := store.PersistString("git.user_email", "config@example.com"); err != nil {
		t.Fatalf("PersistString(user_email) error = %v", err)
	}

	identity := cfg.ResolveGitIdentity(context.Background())
	if got, want := identity.Name, "Config User"; got != want {
		t.Fatalf("Name = %q, want %q", got, want)
	}
	if got, want := identity.Email, "config@example.com"; got != want {
		t.Fatalf("Email = %q, want %q", got, want)
	}
}

func TestResolveGitIdentityReadsGroupedConfig(t *testing.T) {
	cfg, store := newTestConfig(t)
	if err := store.PersistStruct("git", map[string]any{
		"user_name":  "Grouped User",
		"user_email": "grouped@example.com",
	}); err != nil {
		t.Fatalf("PersistStruct(git) error = %v", err)
	}

	identity := cfg.ResolveGitIdentity(context.Background())
	if got, want := identity.Name, "Grouped User"; got != want {
		t.Fatalf("Name = %q, want %q", got, want)
	}
	if got, want := identity.Email, "grouped@example.com"; got != want {
		t.Fatalf("Email = %q, want %q", got, want)
	}
}

func TestGitConfigGettersAndSetters(t *testing.T) {
	cfg, _ := newTestConfig(t)

	if err := cfg.Git().SetUserName(" Config User "); err != nil {
		t.Fatalf("SetUserName() error = %v", err)
	}
	if err := cfg.Git().SetUserEmail(" config@example.com "); err != nil {
		t.Fatalf("SetUserEmail() error = %v", err)
	}

	if got, want := cfg.Git().UserName(), "Config User"; got != want {
		t.Fatalf("UserName() = %q, want %q", got, want)
	}
	if got, want := cfg.Git().UserEmail(), "config@example.com"; got != want {
		t.Fatalf("UserEmail() = %q, want %q", got, want)
	}
}

func TestResolveGitIdentityUsesHostGlobalOnly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	cfg, _ := newTestConfig(t)
	t.Setenv("GIT_CONFIG_GLOBAL", writeGitConfig(t, "Host User", "host@example.com"))

	identity := cfg.ResolveGitIdentity(context.Background())
	if got, want := identity.Name, "Host User"; got != want {
		t.Fatalf("Name = %q, want %q", got, want)
	}
	if got, want := identity.Email, "host@example.com"; got != want {
		t.Fatalf("Email = %q, want %q", got, want)
	}
}

func TestResolveGitIdentityIgnoresRepoLocalGitConfig(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	cfg, _ := newTestConfig(t)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "empty-global-gitconfig"))

	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.name", "Repo Bot")
	runGit(t, repo, "config", "user.email", "repo-bot@example.com")
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("Chdir(repo) error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	if identity := cfg.ResolveGitIdentity(context.Background()); identity.Complete() {
		t.Fatalf("ResolveGitIdentity() = %#v, want no identity from repo-local git config", identity)
	}
}

func TestResolveGitIdentityDoesNotFallbackWhenExplicitConfigIncomplete(t *testing.T) {
	cfg, store := newTestConfig(t)
	t.Setenv("GIT_CONFIG_GLOBAL", writeGitConfig(t, "Host User", "host@example.com"))
	if err := store.PersistString("git.user_name", "Config User"); err != nil {
		t.Fatalf("PersistString(user_name) error = %v", err)
	}

	if identity := cfg.ResolveGitIdentity(context.Background()); identity.Complete() {
		t.Fatalf("ResolveGitIdentity() = %#v, want incomplete identity", identity)
	}
}

func TestGitIdentityEnv(t *testing.T) {
	env := (GitIdentity{Name: "Human", Email: "human@example.com"}).Env()
	for _, want := range []string{
		"GIT_AUTHOR_NAME=Human",
		"GIT_AUTHOR_EMAIL=human@example.com",
		"GIT_COMMITTER_NAME=Human",
		"GIT_COMMITTER_EMAIL=human@example.com",
	} {
		if !containsEnv(env, want) {
			t.Fatalf("Env() missing %q in %#v", want, env)
		}
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func writeGitConfig(t *testing.T, name string, email string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".gitconfig")
	data := "[user]\n\tname = " + name + "\n\temail = " + email + "\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile(.gitconfig) error = %v", err)
	}
	return path
}

func containsEnv(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
