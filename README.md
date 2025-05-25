# kproximate

A node autoscaler project for Proxmox allowing a Kubernetes cluster to dynamically scale across a Proxmox cluster.

- Polls for unschedulable pods
- Assesses the resource requirements from the requests of the unschedulable pods
- Provisions VMs from a user defined template
- User configurable cpu, memory and ssh key for provisioned VMs
- Removes nodes when requested resources can be satisfied by fewer VMs

The operator is required to create a Proxmox template configured to join the kubernetes cluster automatically on it's first boot. All examples show how to do this with K3S, however other kubernetes cluster flavours theoretically could be used if you are prepared to put in the work to build an appropriate template.

While it is a pretty niche project, some possible use cases include:

- Providing overflow capacity for a raspberry Pi cluster
- Running multiple k8s clusters each with fluctuating loads on a single proxmox cluster
- Something someone else thinks of that I haven't
- Just because...

## Credits

This project is based on the original [kproximate](https://github.com/jedrw/kproximate) by [jedrw](https://github.com/jedrw), with some additional improvements.

## Development

For local development without Helm, see the [Development Guide](dev/README.md).

## Configuration and Installation

See [here](https://github.com/paradoxe35/kproximate/tree/main/examples) for example setup scripts and configuration.

## Scaling

Kproximate polls the kubernetes cluster by default every 10 seconds looking for unschedulable resources.

**_Important_**\
Scaling is calculated based on pod requests. Resource requests must be set for all pods in the cluster which are not fixed to control plane nodes else the cluster may be left with continually pending pods.

## Scaling Up

Kproximate will scale upwards as fast as it can provision VMs in the cluster limited by the amount of worker replicas deployed. As soon as unschedulable CPU or memory resource requests are found kproximate will assess the resource requirements and provision Proxmox VMs to satisfy them.

Scaling events are asynchronous so if new resources requests are found on the cluster while a scaling event is in progress then an additional scaling event will be triggered if the initial scaling event will not be able to satisfy the new resource requests.

## Scaling Down

Scaling down is designed to be stable and prevent oscillation between scale-up and scale-down events. The autoscaler includes multiple safety mechanisms to ensure cluster stability:

### Stabilization Mechanisms

**Scale-Down Stabilization Period**

- Prevents scale-down for a configurable period (default: 5 minutes) after any node creation
- Ensures the cluster has time to stabilize after scaling events
- Configurable via `scaleDownStabilizationMinutes`

**Minimum Node Age Protection**

- Individual nodes must reach a minimum age (default: 10 minutes) before being considered for removal
- Prevents newly created nodes from being immediately removed
- Configurable via `minNodeAgeMinutes`

### Scale-Down Assessment

When stabilization periods have passed, the autoscaler evaluates whether scale-down is safe:

1. **Load Calculation**: Assesses if the cluster's resource requirements can be satisfied by removing one kproximate node
2. **Headroom Validation**: Ensures the projected load remains within the configured `loadHeadroom` safety margin
3. **Mixed Environment Support**: Correctly accounts for both existing cluster nodes and kproximate-managed nodes in capacity calculations
4. **Smart Node Selection**: Selects the optimal node for removal based on:
   - Current resource utilization (prefers nodes with lower load)
   - Node capacity (prefers smaller nodes when utilization is similar)
   - Node age (respects minimum age requirements)

### Removal Process

Once a node is selected for scale-down:

1. All pods are gracefully evicted from the target node
2. The node is removed from the Kubernetes cluster
3. The corresponding Proxmox VM is deleted

This approach ensures that scale-down operations are conservative, predictable, and maintain cluster stability while efficiently managing resources.

## Proxmox Host Targeting

Kproximate supports multiple strategies for selecting Proxmox hosts when scaling up. The selection strategy can be configured using the `nodeSelectionStrategy` configuration option.

### Available Strategies

**Spread Strategy (Default)**

- Distributes nodes across different Proxmox hosts
- Skips hosts that already have a kproximate node or pending scaling event
- Falls back to max memory strategy if no empty host is available

**Max Memory Strategy**

- Selects the host with the most available memory
- Ideal for memory-intensive workloads

**Max CPU Strategy**

- Selects the host with the most available CPU (lowest CPU usage)
- Ideal for CPU-intensive workloads

**Balanced Strategy**

- Selects the host with the best combined CPU and memory availability
- Provides optimal resource distribution

**Round Robin Strategy**

- Cycles through available hosts in sequence
- Ensures even distribution across all eligible hosts

### Resource Restrictions

You can configure minimum resource requirements that hosts must meet to be eligible for scaling:

- `minAvailableCpuCores`: Minimum available CPU cores required
- `minAvailableMemoryMB`: Minimum available memory in MB required

Hosts that don't meet these requirements will be excluded from selection.

### Node Exclusion

You can exclude specific Proxmox nodes from scaling operations using the `excludedNodes` configuration option. This is useful for:

- Nodes under maintenance
- Nodes dedicated to specific workloads
- Nodes with resource constraints

Example: `excludedNodes: "proxmox-node-1,proxmox-node-maintenance"`

## Node Labels

Nodes can labeled with dynamic values only known at provisioning time using go templating language in a configuration option. Currently this is limited to a single templatable value `TargetHost` which is the name of the proxmox host that the kproximate node will be provisioned on. More options may be added in the future as more use cases appear. See [example-values.yaml](https://github.com/paradoxe35/kproximate/tree/main/examples/example-values.yaml) for an example.

## Metrics

A metrics endpoint is provided at `kproximate.kproximate.cluster.svc.local/metrics` by default. The following metrics are provided in Prometheus format with CPU measured in [CPU units](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#meaning-of-cpu) and memory measured in bytes:

`kpnodes_total`
<br>
The total number of kproximate nodes

`kpnodes_running`
<br>
The total number of running kproximate nodes

`cpu_provisioned_total`
<br>
The total provisioned cpu

`memory_provisioned_total`
<br>
The total memory provisioned

`cpu_allocatable_total`
<br>
The total cpu allocatable

`memory_allocatable_total`
<br>
The total memory allocatable

`cpu_allocated_total`
<br>
The total cpu allocated

`memory_allocated_total`
<br>
The total memory allocated
