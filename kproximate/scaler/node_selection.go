package scaler

import (
	"fmt"
	"sort"
	"strings"

	"github.com/paradoxe35/kproximate/config"
	"github.com/paradoxe35/kproximate/proxmox"
)

// NodeSelector handles different strategies for selecting Proxmox hosts for scaling up
type NodeSelector struct {
	config          config.KproximateConfig
	excludedNodes   map[string]bool
	roundRobinIndex int
}

// NewNodeSelector creates a new NodeSelector with the given configuration
func NewNodeSelector(cfg config.KproximateConfig) *NodeSelector {
	excludedNodes := make(map[string]bool)
	if cfg.ExcludedNodes != "" {
		nodes := strings.Split(cfg.ExcludedNodes, ",")
		for _, node := range nodes {
			excludedNodes[strings.TrimSpace(node)] = true
		}
	}

	return &NodeSelector{
		config:        cfg,
		excludedNodes: excludedNodes,
	}
}

// SelectTargetHost selects the best Proxmox host based on the configured strategy
func (ns *NodeSelector) SelectTargetHost(hosts []proxmox.HostInformation, kpNodes []proxmox.VmInformation, scaleEvents []*ScaleEvent) proxmox.HostInformation {
	// Filter hosts based on resource requirements and exclusions
	eligibleHosts := ns.filterEligibleHosts(hosts, kpNodes, scaleEvents)

	if len(eligibleHosts) == 0 {
		// If no hosts meet the criteria, fall back to the original logic
		// but still respect exclusions
		eligibleHosts = ns.filterExcludedHosts(hosts)
		if len(eligibleHosts) == 0 {
			// If all hosts are excluded, return the first available host
			return hosts[0]
		}
	}

	switch ns.config.NodeSelectionStrategy {
	case "max-memory":
		return ns.selectMaxMemoryHost(eligibleHosts)
	case "max-cpu":
		return ns.selectMaxCpuHost(eligibleHosts)
	case "balanced":
		return ns.selectBalancedHost(eligibleHosts)
	case "round-robin":
		return ns.selectRoundRobinHost(eligibleHosts)
	case "spread":
		fallthrough
	default:
		return ns.selectSpreadHost(eligibleHosts, kpNodes, scaleEvents)
	}
}

// filterEligibleHosts filters hosts based on resource requirements, exclusions, and existing nodes
func (ns *NodeSelector) filterEligibleHosts(hosts []proxmox.HostInformation, kpNodes []proxmox.VmInformation, scaleEvents []*ScaleEvent) []proxmox.HostInformation {
	var eligible []proxmox.HostInformation

hostLoop:
	for _, host := range hosts {
		// Skip excluded nodes
		if ns.excludedNodes[host.Node] {
			continue
		}

		// Skip if host doesn't meet minimum resource requirements
		if !ns.meetsResourceRequirements(host) {
			continue
		}

		// Check for existing scaling events targeting this host
		for _, scaleEvent := range scaleEvents {
			if scaleEvent.TargetHost.Node == host.Node {
				continue hostLoop
			}
		}

		eligible = append(eligible, host)
	}

	return eligible
}

// filterExcludedHosts filters out only excluded hosts (used as fallback)
func (ns *NodeSelector) filterExcludedHosts(hosts []proxmox.HostInformation) []proxmox.HostInformation {
	var eligible []proxmox.HostInformation

	for _, host := range hosts {
		if !ns.excludedNodes[host.Node] {
			eligible = append(eligible, host)
		}
	}

	return eligible
}

// meetsResourceRequirements checks if a host meets the minimum resource requirements
func (ns *NodeSelector) meetsResourceRequirements(host proxmox.HostInformation) bool {
	// Calculate available resources
	availableMemoryMB := (host.Maxmem - host.Mem) / (1024 * 1024) // Convert bytes to MB

	// Calculate available CPU cores using the actual Maxcpu field
	// host.Cpu is a percentage (0.0 to 1.0) of total CPU usage
	// host.Maxcpu is the total number of CPU cores available on the host
	availableCores := int(float64(host.Maxcpu) * (1.0 - host.Cpu))

	return availableCores >= ns.config.MinAvailableCpuCores &&
		int(availableMemoryMB) >= ns.config.MinAvailableMemoryMB
}

// selectSpreadHost implements the original spread strategy
func (ns *NodeSelector) selectSpreadHost(hosts []proxmox.HostInformation, kpNodes []proxmox.VmInformation, scaleEvents []*ScaleEvent) proxmox.HostInformation {
skipHost:
	for _, host := range hosts {
		// Check for existing kpNode on the host
		for _, kpNode := range kpNodes {
			if kpNode.Node == host.Node {
				continue skipHost
			}
		}

		return host
	}

	// If no empty host found, fall back to max memory strategy
	return ns.selectMaxMemoryHost(hosts)
}

// selectMaxMemoryHost selects the host with the most available memory
func (ns *NodeSelector) selectMaxMemoryHost(hosts []proxmox.HostInformation) proxmox.HostInformation {
	if len(hosts) == 0 {
		return proxmox.HostInformation{}
	}

	selectedHost := hosts[0]
	maxAvailableMemory := selectedHost.Maxmem - selectedHost.Mem

	for _, host := range hosts[1:] {
		availableMemory := host.Maxmem - host.Mem
		if availableMemory > maxAvailableMemory {
			selectedHost = host
			maxAvailableMemory = availableMemory
		}
	}

	return selectedHost
}

// selectMaxCpuHost selects the host with the lowest CPU usage (most available CPU)
func (ns *NodeSelector) selectMaxCpuHost(hosts []proxmox.HostInformation) proxmox.HostInformation {
	if len(hosts) == 0 {
		return proxmox.HostInformation{}
	}

	selectedHost := hosts[0]
	lowestCpuUsage := selectedHost.Cpu

	for _, host := range hosts[1:] {
		if host.Cpu < lowestCpuUsage {
			selectedHost = host
			lowestCpuUsage = host.Cpu
		}
	}

	return selectedHost
}

// selectBalancedHost selects the host with the best combined CPU and memory availability
func (ns *NodeSelector) selectBalancedHost(hosts []proxmox.HostInformation) proxmox.HostInformation {
	if len(hosts) == 0 {
		return proxmox.HostInformation{}
	}

	type hostScore struct {
		host  proxmox.HostInformation
		score float64
	}

	var scoredHosts []hostScore

	for _, host := range hosts {
		// Calculate normalized scores (0-1) for CPU and memory availability
		cpuAvailability := 1.0 - host.Cpu // Lower CPU usage = higher availability
		memoryAvailability := float64(host.Maxmem-host.Mem) / float64(host.Maxmem)

		// Combined score (equal weight for CPU and memory)
		combinedScore := (cpuAvailability + memoryAvailability) / 2.0

		scoredHosts = append(scoredHosts, hostScore{
			host:  host,
			score: combinedScore,
		})
	}

	// Sort by score (highest first)
	sort.Slice(scoredHosts, func(i, j int) bool {
		return scoredHosts[i].score > scoredHosts[j].score
	})

	return scoredHosts[0].host
}

// selectRoundRobinHost selects hosts in a round-robin fashion
func (ns *NodeSelector) selectRoundRobinHost(hosts []proxmox.HostInformation) proxmox.HostInformation {
	if len(hosts) == 0 {
		return proxmox.HostInformation{}
	}

	selectedHost := hosts[ns.roundRobinIndex%len(hosts)]
	ns.roundRobinIndex++

	return selectedHost
}

// GetStrategyDescription returns a human-readable description of the current strategy
func (ns *NodeSelector) GetStrategyDescription() string {
	descriptions := map[string]string{
		"spread":      "Spread Strategy - Distributes nodes across different Proxmox hosts",
		"max-memory":  "Max Memory Strategy - Selects host with most available memory",
		"max-cpu":     "Max CPU Strategy - Selects host with most available CPU",
		"balanced":    "Balanced Strategy - Selects host with best combined CPU and memory availability",
		"round-robin": "Round Robin Strategy - Cycles through available hosts",
	}

	description := descriptions[ns.config.NodeSelectionStrategy]
	if description == "" {
		description = descriptions["spread"] // fallback
	}

	if ns.config.MinAvailableCpuCores > 0 || ns.config.MinAvailableMemoryMB > 0 {
		description += fmt.Sprintf(" (Min requirements: %d CPU cores, %d MB memory)",
			ns.config.MinAvailableCpuCores, ns.config.MinAvailableMemoryMB)
	}

	if len(ns.excludedNodes) > 0 {
		var excludedList []string
		for node := range ns.excludedNodes {
			excludedList = append(excludedList, node)
		}
		description += fmt.Sprintf(" (Excluded nodes: %s)", strings.Join(excludedList, ", "))
	}

	return description
}
