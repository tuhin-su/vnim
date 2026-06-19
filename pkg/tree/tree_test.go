package tree

import (
	"strings"
	"testing"

	"github.com/tuhin-su/vnim/pkg/config"
)

func TestTreeBuilding(t *testing.T) {
	cfg := &config.Config{
		Objects: []config.Object{
			{Type: "namespace", Name: "ns0"},
			{Type: "bridge", Name: "br0", Namespace: "ns0"},
			{Type: "tap", Name: "tap0", Master: "br0", Namespace: "ns0"},
			{Type: "address", Interface: "br0", Address: "192.168.100.1/24"},
		},
	}

	roots := BuildTree(cfg)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root node (namespace ns0), got %d", len(roots))
	}

	root := roots[0]
	if root.Name != "ns0" || root.Type != "namespace" {
		t.Errorf("expected root name 'ns0' and type 'namespace', got %s (%s)", root.Name, root.Type)
	}

	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child of ns0 (bridge br0), got %d", len(root.Children))
	}

	bridge := root.Children[0]
	if bridge.Name != "br0" || bridge.Type != "bridge" {
		t.Errorf("expected bridge 'br0', got %s (%s)", bridge.Name, bridge.Type)
	}

	if !strings.Contains(bridge.Details, "ip: 192.168.100.1/24") {
		t.Errorf("expected bridge details to contain IP address, got %q", bridge.Details)
	}

	if len(bridge.Children) != 1 {
		t.Fatalf("expected 1 child of bridge (tap0), got %d", len(bridge.Children))
	}

	tap := bridge.Children[0]
	if tap.Name != "tap0" || tap.Type != "tap" {
		t.Errorf("expected tap 'tap0', got %s (%s)", tap.Name, tap.Type)
	}

	rendered := RenderTree(roots)
	expectedTree := "└── ns0 (namespace)\n    └── br0 (bridge) [ip: 192.168.100.1/24]\n        └── tap0 (tap)\n"
	if rendered != expectedTree {
		t.Errorf("rendered tree output mismatch:\nExpected:\n%s\nGot:\n%s", expectedTree, rendered)
	}
}
