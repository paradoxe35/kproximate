# Proxmox Node Selection Strategy Improvements

## Overview

This document describes the improvements made to the Proxmox node selection strategy for scaling up operations in kproximate. The changes provide multiple selection strategies, resource restrictions, and node exclusion capabilities.

## Previous Implementation

The original implementation used a simple strategy:
1. **Spread Logic**: Skip hosts that already have a kproximate node or pending scale event
2. **Fallback**: If no empty host is available, select the host with the most available memory

## New Implementation

### Multiple Selection Strategies

The new implementation supports five different strategies:

#### 1. Spread Strategy (Default)
- **Name**: `spread`
- **Description**: Distributes nodes across different Proxmox hosts
- **Behavior**: 
  - Prefers hosts without existing kproximate nodes
  - Skips hosts with pending scaling events
  - Falls back to max memory strategy when no empty hosts are available
- **Use Case**: Default behavior, good for general workloads

#### 2. Max Memory Strategy
- **Name**: `max-memory`
- **Description**: Selects host with most available memory
- **Behavior**: Always selects the host with the highest available memory
- **Use Case**: Memory-intensive workloads

#### 3. Max CPU Strategy
- **Name**: `max-cpu`
- **Description**: Selects host with most available CPU
- **Behavior**: Selects the host with the lowest CPU usage (most available CPU)
- **Use Case**: CPU-intensive workloads

#### 4. Balanced Strategy
- **Name**: `balanced`
- **Description**: Selects host with best combined CPU and memory availability
- **Behavior**: Calculates a combined score based on both CPU and memory availability
- **Use Case**: Workloads that need balanced resource distribution

#### 5. Round Robin Strategy
- **Name**: `round-robin`
- **Description**: Cycles through available hosts in sequence
- **Behavior**: Ensures even distribution across all eligible hosts
- **Use Case**: When you want guaranteed even distribution

### Resource Restrictions

You can now configure minimum resource requirements that hosts must meet to be eligible for scaling:

- **`minAvailableCpuCores`**: Minimum available CPU cores required (default: 0)
- **`minAvailableMemoryMB`**: Minimum available memory in MB required (default: 0)

Hosts that don't meet these requirements will be excluded from selection.

### Node Exclusion

You can exclude specific Proxmox nodes from scaling operations using the `excludedNodes` configuration option:

- **Format**: Comma-separated list of node names
- **Example**: `"proxmox-node-1,proxmox-node-maintenance"`
- **Use Cases**:
  - Nodes under maintenance
  - Nodes dedicated to specific workloads
  - Nodes with resource constraints

## Configuration

### Environment Variables

```bash
# Node selection strategy
nodeSelectionStrategy=balanced

# Resource restrictions
minAvailableCpuCores=2
minAvailableMemoryMB=8192

# Node exclusion
excludedNodes="node1,node2"
```

### Helm Chart Values

```yaml
kproximate:
  config:
    nodeSelectionStrategy: "balanced"
    minAvailableCpuCores: 2
    minAvailableMemoryMB: 8192
    excludedNodes: "node1,node2"
```

## Examples

### Example 1: Memory-Intensive Workloads
```yaml
kproximate:
  config:
    nodeSelectionStrategy: "max-memory"
    minAvailableMemoryMB: 16384  # 16GB minimum
    excludedNodes: "small-node-1,small-node-2"
```

### Example 2: CPU-Intensive Workloads
```yaml
kproximate:
  config:
    nodeSelectionStrategy: "max-cpu"
    minAvailableCpuCores: 4
    excludedNodes: "gpu-node-1,gpu-node-2"
```

### Example 3: Balanced Distribution
```yaml
kproximate:
  config:
    nodeSelectionStrategy: "balanced"
    minAvailableCpuCores: 2
    minAvailableMemoryMB: 8192
```

### Example 4: Even Distribution
```yaml
kproximate:
  config:
    nodeSelectionStrategy: "round-robin"
    minAvailableCpuCores: 1
    minAvailableMemoryMB: 4096
```

## Implementation Details

### Files Modified

1. **`kproximate/config/config.go`**: Added new configuration fields
2. **`kproximate/scaler/node_selection.go`**: New file with selection strategies
3. **`kproximate/scaler/proxmox.go`**: Updated to use new node selector
4. **`chart/kproximate/values.yaml`**: Added new configuration options
5. **`chart/kproximate/templates/configmap.yaml`**: Added new environment variables
6. **`README.md`**: Updated documentation

### Files Added

1. **`kproximate/scaler/node_selection.go`**: Core node selection logic
2. **`kproximate/scaler/node_selection_test.go`**: Comprehensive tests
3. **`examples/node-selection-strategies-values.yaml`**: Example configurations

### Backward Compatibility

The implementation maintains full backward compatibility:
- Default strategy is `spread` (matches original behavior)
- All new configuration options have sensible defaults
- Existing configurations will continue to work without changes

### Testing

Comprehensive test suite includes:
- Unit tests for each selection strategy
- Resource restriction testing
- Node exclusion testing
- Round-robin behavior verification
- Integration tests with existing functionality

## Benefits

1. **Flexibility**: Multiple strategies for different workload types
2. **Resource Management**: Prevent overloading hosts with resource restrictions
3. **Maintenance Support**: Exclude nodes during maintenance or for dedicated workloads
4. **Better Resource Utilization**: Strategies optimized for specific resource patterns
5. **Operational Control**: Fine-grained control over node selection behavior

## Migration Guide

### For Existing Users

No action required - the default behavior remains the same.

### For New Features

1. Choose appropriate strategy based on workload characteristics
2. Set resource restrictions based on your infrastructure capacity
3. Configure node exclusions for operational requirements
4. Monitor logs for strategy selection information

## Logging

The system now logs the selected strategy and its configuration:

```
INFO Using node selection strategy: Balanced Strategy - Selects host with best combined CPU and memory availability (Min requirements: 2 CPU cores, 8192 MB memory) (Excluded nodes: node1, node2)
```

This helps with debugging and operational visibility.
