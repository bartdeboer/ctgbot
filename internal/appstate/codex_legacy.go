package appstate

// legacyProfileHostPathOverride reads pre-profile-rename Codex config keys.
// Keep this isolated so current code can use profile terminology everywhere
// else while still upgrading existing operator config safely.
func (c CodexConfig) legacyProfileHostPathOverride() string {
	for _, key := range []string{"codex.cli_home_host_path", "codex.shared_home_host_path"} {
		if raw := absOrEmpty(c.cfg.string(key, "")); raw != "" {
			return raw
		}
	}
	return ""
}
