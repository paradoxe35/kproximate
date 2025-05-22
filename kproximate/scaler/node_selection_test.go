package scaler

import (
	"testing"

	"github.com/paradoxe35/kproximate/config"
	"github.com/paradoxe35/kproximate/proxmox"
)

func TestNewNodeSelector(t *testing.T) {
	cfg := config.KproximateConfig{
		NodeSelectionStrategy: "spread",
		ExcludedNodes:         "node1,node2,node3",
	}

	selector := NewNodeSelector(cfg)

	if selector.config.NodeSelectionStrategy != "spread" {
		t.Errorf("Expected strategy 'spread', got '%s'", selector.config.NodeSelectionStrategy)
	}

	if !selector.excludedNodes["node1"] || !selector.excludedNodes["node2"] || !selector.excludedNodes["node3"] {
		t.Error("Expected excluded nodes to be parsed correctly")
	}

	if selector.excludedNodes["node4"] {
		t.Error("Expected node4 to not be excluded")
	}
}

func TestSelectTargetHost_SpreadStrategy(t *testing.T) {
	cfg := config.KproximateConfig{
		NodeSelectionStrategy: "spread",
		MinAvailableCpuCores:  0,
		MinAvailableMemoryMB:  0,
		ExcludedNodes:         "",
	}

	selector := NewNodeSelector(cfg)

	hosts := []proxmox.HostInformation{
		{Node: "host1", Cpu: 0.2, Mem: 8000000000, Maxmem: 16000000000},
		{Node: "host2", Cpu: 0.3, Mem: 12000000000, Maxmem: 16000000000},
		{Node: "host3", Cpu: 0.1, Mem: 4000000000, Maxmem: 16000000000},
	}

	kpNodes := []proxmox.VmInformation{
		{Node: "host1", Name: "kp-node-1"},
	}

	scaleEvents := []*ScaleEvent{}

	selectedHost := selector.SelectTargetHost(hosts, kpNodes, scaleEvents)

	// Should select host2 or host3 (not host1 which has existing kpNode)
	if selectedHost.Node == "host1" {
		t.Error("Expected to skip host1 which already has a kpNode")
	}
}

func TestSelectTargetHost_MaxMemoryStrategy(t *testing.T) {
	cfg := config.KproximateConfig{
		NodeSelectionStrategy: "max-memory",
		MinAvailableCpuCores:  0,
		MinAvailableMemoryMB:  0,
		ExcludedNodes:         "",
	}

	selector := NewNodeSelector(cfg)

	hosts := []proxmox.HostInformation{
		{Node: "host1", Cpu: 0.2, Mem: 8000000000, Maxmem: 16000000000},  // 8GB available
		{Node: "host2", Cpu: 0.3, Mem: 4000000000, Maxmem: 16000000000},  // 12GB available
		{Node: "host3", Cpu: 0.1, Mem: 12000000000, Maxmem: 16000000000}, // 4GB available
	}

	kpNodes := []proxmox.VmInformation{}
	scaleEvents := []*ScaleEvent{}

	selectedHost := selector.SelectTargetHost(hosts, kpNodes, scaleEvents)

	// Should select host2 which has the most available memory (12GB)
	if selectedHost.Node != "host2" {
		t.Errorf("Expected host2 (most available memory), got %s", selectedHost.Node)
	}
}

func TestSelectTargetHost_MaxCpuStrategy(t *testing.T) {
	cfg := config.KproximateConfig{
		NodeSelectionStrategy: "max-cpu",
		MinAvailableCpuCores:  0,
		MinAvailableMemoryMB:  0,
		ExcludedNodes:         "",
	}

	selector := NewNodeSelector(cfg)

	hosts := []proxmox.HostInformation{
		{Node: "host1", Cpu: 0.8, Mem: 8000000000, Maxmem: 16000000000},  // High CPU usage
		{Node: "host2", Cpu: 0.1, Mem: 4000000000, Maxmem: 16000000000},  // Low CPU usage
		{Node: "host3", Cpu: 0.5, Mem: 12000000000, Maxmem: 16000000000}, // Medium CPU usage
	}

	kpNodes := []proxmox.VmInformation{}
	scaleEvents := []*ScaleEvent{}

	selectedHost := selector.SelectTargetHost(hosts, kpNodes, scaleEvents)

	// Should select host2 which has the lowest CPU usage
	if selectedHost.Node != "host2" {
		t.Errorf("Expected host2 (lowest CPU usage), got %s", selectedHost.Node)
	}
}

func TestSelectTargetHost_ExcludedNodes(t *testing.T) {
	cfg := config.KproximateConfig{
		NodeSelectionStrategy: "spread",
		MinAvailableCpuCores:  0,
		MinAvailableMemoryMB:  0,
		ExcludedNodes:         "host1,host3",
	}

	selector := NewNodeSelector(cfg)

	hosts := []proxmox.HostInformation{
		{Node: "host1", Cpu: 0.2, Mem: 8000000000, Maxmem: 16000000000},
		{Node: "host2", Cpu: 0.3, Mem: 12000000000, Maxmem: 16000000000},
		{Node: "host3", Cpu: 0.1, Mem: 4000000000, Maxmem: 16000000000},
	}

	kpNodes := []proxmox.VmInformation{}
	scaleEvents := []*ScaleEvent{}

	selectedHost := selector.SelectTargetHost(hosts, kpNodes, scaleEvents)

	// Should only select host2 since host1 and host3 are excluded
	if selectedHost.Node != "host2" {
		t.Errorf("Expected host2 (only non-excluded host), got %s", selectedHost.Node)
	}
}

func TestSelectTargetHost_ResourceRequirements(t *testing.T) {
	cfg := config.KproximateConfig{
		NodeSelectionStrategy: "spread",
		MinAvailableCpuCores:  4,
		MinAvailableMemoryMB:  8192, // 8GB
		ExcludedNodes:         "",
	}

	selector := NewNodeSelector(cfg)

	hosts := []proxmox.HostInformation{
		// host1: Low available memory (4GB), should be filtered out
		{Node: "host1", Cpu: 0.2, Mem: 12000000000, Maxmem: 16000000000},
		// host2: Good available memory (12GB), should be eligible
		{Node: "host2", Cpu: 0.3, Mem: 4000000000, Maxmem: 16000000000},
		// host3: Medium available memory (8GB), should be eligible
		{Node: "host3", Cpu: 0.1, Mem: 8000000000, Maxmem: 16000000000},
	}

	kpNodes := []proxmox.VmInformation{}
	scaleEvents := []*ScaleEvent{}

	selectedHost := selector.SelectTargetHost(hosts, kpNodes, scaleEvents)

	// Should select host2 or host3, but not host1 (insufficient memory)
	if selectedHost.Node == "host1" {
		t.Error("Expected to skip host1 which doesn't meet memory requirements")
	}
}

func TestSelectTargetHost_RoundRobin(t *testing.T) {
	cfg := config.KproximateConfig{
		NodeSelectionStrategy: "round-robin",
		MinAvailableCpuCores:  0,
		MinAvailableMemoryMB:  0,
		ExcludedNodes:         "",
	}

	selector := NewNodeSelector(cfg)

	hosts := []proxmox.HostInformation{
		{Node: "host1", Cpu: 0.2, Mem: 8000000000, Maxmem: 16000000000},
		{Node: "host2", Cpu: 0.3, Mem: 12000000000, Maxmem: 16000000000},
		{Node: "host3", Cpu: 0.1, Mem: 4000000000, Maxmem: 16000000000},
	}

	kpNodes := []proxmox.VmInformation{}
	scaleEvents := []*ScaleEvent{}

	// Test multiple selections to verify round-robin behavior
	first := selector.SelectTargetHost(hosts, kpNodes, scaleEvents)
	second := selector.SelectTargetHost(hosts, kpNodes, scaleEvents)
	third := selector.SelectTargetHost(hosts, kpNodes, scaleEvents)
	fourth := selector.SelectTargetHost(hosts, kpNodes, scaleEvents)

	// Should cycle through hosts
	if first.Node == second.Node && second.Node == third.Node {
		t.Error("Round-robin should select different hosts in sequence")
	}

	// Fourth selection should wrap around to first host
	if first.Node != fourth.Node {
		t.Error("Round-robin should wrap around after cycling through all hosts")
	}
}

func TestGetStrategyDescription(t *testing.T) {
	cfg := config.KproximateConfig{
		NodeSelectionStrategy: "balanced",
		MinAvailableCpuCores:  2,
		MinAvailableMemoryMB:  4096,
		ExcludedNodes:         "node1,node2",
	}

	selector := NewNodeSelector(cfg)
	description := selector.GetStrategyDescription()

	// Should contain strategy name, resource requirements, and excluded nodes
	if description == "" {
		t.Error("Expected non-empty strategy description")
	}

	// Basic checks for content
	expectedSubstrings := []string{"Balanced Strategy", "2 CPU cores", "4096 MB memory", "node1", "node2"}
	for _, substr := range expectedSubstrings {
		if !contains(description, substr) {
			t.Errorf("Expected description to contain '%s', got: %s", substr, description)
		}
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || 
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || 
		containsInMiddle(s, substr))))
}

func containsInMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
