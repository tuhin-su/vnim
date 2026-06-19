package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tuhin-su/vnim/pkg/config"
)

func TestPluginExecution(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vnim-services-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sm := &ServiceManager{
		planName: "test-plan",
		stateDir: tempDir,
	}

	ctx := templateCtx{
		Interface: "dummy0",
		Namespace: "test-ns",
		Mac:       "00:11:22:33:44:55",
		IP:        "192.168.1.1/24",
		IPNoMask:  "192.168.1.1",
		PlanName:  "test-plan",
		StateDir:  tempDir,
		Owner:     "test-owner",
	}

	tmpl := "interface: {{.Interface}}, ns: {{.Namespace}}, mac: {{.Mac}}, ip: {{.IP}}, ip_nomask: {{.IPNoMask}}, plan: {{.PlanName}}, state: {{.StateDir}}, owner: {{.Owner}}"
	expected := "interface: dummy0, ns: test-ns, mac: 00:11:22:33:44:55, ip: 192.168.1.1/24, ip_nomask: 192.168.1.1, plan: test-plan, state: " + tempDir + ", owner: test-owner"

	res, err := compileTemplate(tmpl, ctx)
	if err != nil {
		t.Fatalf("failed to compile template: %v", err)
	}
	if res != expected {
		t.Errorf("expected %q, got %q", expected, res)
	}

	// Test writing and executing a simple script
	svc := config.Service{
		Type:   "plugin",
		Mode:   "su-exec",
		Script: "echo 'hello from plugin' > " + filepath.Join(tempDir, "output.txt"),
	}

	scriptPath, err := sm.compileAndWriteScript(svc.Script, "test_plugin.sh", ctx)
	if err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	logPath := filepath.Join(tempDir, "test_plugin.log")
	pid, err := runPluginCmd(svc.Mode, "root", scriptPath, nil, logPath, true, "")
	if err != nil {
		t.Fatalf("failed to run plugin cmd: %v", err)
	}
	if pid != 0 {
		t.Errorf("expected pid 0 for synchronous wait execution, got %d", pid)
	}

	// Verify file was written
	outBytes, err := os.ReadFile(filepath.Join(tempDir, "output.txt"))
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if strings.TrimSpace(string(outBytes)) != "hello from plugin" {
		t.Errorf("expected output 'hello from plugin', got %q", string(outBytes))
	}
}
