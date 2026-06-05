package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.repairman.native-db-bridge</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{.BinaryPath}}</string>
		<string>serve</string>
		<string>--home</string>
		<string>{{.HomePath}}</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>Crashed</key>
		<true/>
	</dict>
	<key>StandardOutPath</key>
	<string>{{.LogDir}}/stdout.log</string>
	<key>StandardErrorPath</key>
	<string>{{.LogDir}}/stderr.log</string>
</dict>
</plist>
`

const plistLabel = "com.repairman.native-db-bridge"

// RenderLaunchdPlist generates a macOS LaunchAgent plist XML string.
// The plist contains only the binary path, home argument, and log redirects.
// No secrets are included.
func RenderLaunchdPlist(binaryPath, homePath string) string {
	logDir := filepath.Join(homePath, "log")

	t := template.Must(template.New("plist").Parse(plistTemplate))

	data := struct {
		BinaryPath string
		HomePath   string
		LogDir     string
	}{
		BinaryPath: binaryPath,
		HomePath:   homePath,
		LogDir:     logDir,
	}

	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("plist template execution failed: %v", err))
	}

	return buf.String()
}

// plistPath returns the standard macOS LaunchAgent plist path for this service.
func plistPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Sprintf("cannot determine home directory: %v", err))
	}
	return filepath.Join(home, "Library", "LaunchAgents", plistLabel+".plist")
}

// InstallService writes the launchd plist and loads the service.
// It requires homePath (for --home arg and logs) and binaryPath (the MCP binary).
func InstallService(homePath, binaryPath string) error {
	// Ensure log directory exists.
	logDir := filepath.Join(homePath, "log")
	if err := os.MkdirAll(logDir, 0750); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Ensure LaunchAgents directory exists.
	agentsDir := filepath.Dir(plistPath())
	if err := os.MkdirAll(agentsDir, 0750); err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %w", err)
	}

	// Render and write plist.
	content := RenderLaunchdPlist(binaryPath, homePath)
	if err := os.WriteFile(plistPath(), []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write plist: %w", err)
	}

	// Load the service via launchctl.
	if err := exec.Command("launchctl", "load", plistPath()).Run(); err != nil {
		return fmt.Errorf("failed to load service: %w", err)
	}

	return nil
}

// UninstallService unloads the launchd service and removes the plist.
func UninstallService() error {
	p := plistPath()

	// Unload if present (ignore error if not loaded).
	_ = exec.Command("launchctl", "unload", p).Run()

	// Remove plist file if it exists.
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plist: %w", err)
	}

	return nil
}
