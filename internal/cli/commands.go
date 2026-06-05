package cli

// CommandNames returns the list of available CLI commands.
func CommandNames() []string {
	return []string{"serve", "healthcheck", "install-service", "uninstall-service"}
}
