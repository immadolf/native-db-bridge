package cli

import (
	"strings"
	"testing"
)

func TestLaunchdPlistContainsHomeAndBinary(t *testing.T) {
	got := RenderLaunchdPlist("/bin/native-db-bridge-mcp", "/Users/repairman/opt/native-db-bridge/var")
	if !strings.Contains(got, "/bin/native-db-bridge-mcp") {
		t.Fatalf("binary missing")
	}
	if !strings.Contains(got, "/Users/repairman/opt/native-db-bridge/var") {
		t.Fatalf("home missing")
	}
	if strings.Contains(got, "password") {
		t.Fatalf("plist must not contain secrets")
	}
}

func TestLaunchdPlistContainsLabel(t *testing.T) {
	got := RenderLaunchdPlist("/usr/local/bin/mcp", "/tmp/var")
	if !strings.Contains(got, "com.repairman.native-db-bridge") {
		t.Fatalf("label missing from plist")
	}
}

func TestLaunchdPlistContainsServeCommand(t *testing.T) {
	got := RenderLaunchdPlist("/usr/local/bin/mcp", "/tmp/var")
	if !strings.Contains(got, "<string>serve</string>") {
		t.Fatalf("serve argument missing from plist")
	}
	if !strings.Contains(got, "<string>--home</string>") {
		t.Fatalf("--home argument missing from plist")
	}
}

func TestLaunchdPlistContainsLogPaths(t *testing.T) {
	got := RenderLaunchdPlist("/usr/local/bin/mcp", "/tmp/var")
	if !strings.Contains(got, "/tmp/var/log/stdout.log") {
		t.Fatalf("stdout log path missing")
	}
	if !strings.Contains(got, "/tmp/var/log/stderr.log") {
		t.Fatalf("stderr log path missing")
	}
}

func TestLaunchdPlistRunAtLoad(t *testing.T) {
	got := RenderLaunchdPlist("/usr/local/bin/mcp", "/tmp/var")
	if !strings.Contains(got, "<key>RunAtLoad</key>") {
		t.Fatalf("RunAtLoad key missing")
	}
}

func TestLaunchdPlistNoSecrets(t *testing.T) {
	secretKeywords := []string{"password", "token", "secret", "credential", "dsn", "uri"}
	got := RenderLaunchdPlist("/usr/local/bin/mcp", "/tmp/var")
	lower := strings.ToLower(got)
	for _, kw := range secretKeywords {
		if strings.Contains(lower, kw) {
			t.Fatalf("plist must not contain secret keyword %q", kw)
		}
	}
}

func TestLaunchdPlistIsValidXML(t *testing.T) {
	got := RenderLaunchdPlist("/usr/local/bin/mcp", "/tmp/var")
	if !strings.HasPrefix(got, "<?xml") {
		t.Fatalf("plist should start with XML declaration")
	}
	if !strings.Contains(got, "<!DOCTYPE plist") {
		t.Fatalf("plist should contain DOCTYPE")
	}
	if !strings.Contains(got, "</plist>") {
		t.Fatalf("plist should be closed")
	}
}
