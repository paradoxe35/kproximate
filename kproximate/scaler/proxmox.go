package scaler

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/paradoxe35/kproximate/config"
	"github.com/paradoxe35/kproximate/kubernetes"
	"github.com/paradoxe35/kproximate/logger"
	"github.com/paradoxe35/kproximate/proxmox"
	"k8s.io/apimachinery/pkg/util/uuid"
)

type ProxmoxScaler struct {
	config       config.KproximateConfig
	Kubernetes   kubernetes.Kubernetes
	Proxmox      proxmox.Proxmox
	nodeSelector *NodeSelector
}

func NewProxmoxScaler(config config.KproximateConfig) (Scaler, error) {
	kubernetes, err := kubernetes.NewKubernetesClient()
	if err != nil {
		return nil, err
	}

	proxmox, err := proxmox.NewProxmoxClient(config.PmUrl, config.PmAllowInsecure, config.PmUserID, config.PmToken, config.PmPassword, config.PmDebug)
	if err != nil {
		return nil, err
	}

	config.KpNodeNameRegex = *regexp.MustCompile(fmt.Sprintf(`^%s-\w{8}-\w{4}-\w{4}-\w{4}-\w{12}$`, config.KpNodeNamePrefix))

	config.KpNodeParams = map[string]interface{}{
		"agent":     "enabled=1",
		"balloon":   0,
		"cores":     config.KpNodeCores,
		"ipconfig0": "ip=dhcp",
		"memory":    config.KpNodeMemory,
		"onboot":    1,
	}

	if !config.KpNodeDisableSsh {
		config.KpNodeParams["sshkeys"] = strings.Replace(url.QueryEscape(config.SshKey), "+", "%20", 1)
	}

	scaler := ProxmoxScaler{
		config:       config,
		Kubernetes:   &kubernetes,
		Proxmox:      &proxmox,
		nodeSelector: NewNodeSelector(config),
	}

	return &scaler, err
}

func (scaler *ProxmoxScaler) newKpNodeName() string {
	return fmt.Sprintf("%s-%s", scaler.config.KpNodeNamePrefix, uuid.NewUUID())
}

func (scaler *ProxmoxScaler) requiredScaleEvents(requiredResources kubernetes.UnschedulableResources, numCurrentEvents int) ([]*ScaleEvent, error) {
	requiredScaleEvents := []*ScaleEvent{}
	var numCpuNodesRequired int
	var numMemoryNodesRequired int

	if requiredResources.Cpu != 0 {
		// The expected cpu resources after in-progress scaling events complete
		expectedCpu := float64(scaler.config.KpNodeCores) * float64(numCurrentEvents)
		// The expected amount of cpu resources still required after in-progress scaling events complete
		unaccountedCpu := requiredResources.Cpu - expectedCpu
		// The least amount of nodes that will satisfy the unaccountedMemory
		numCpuNodesRequired = int(math.Ceil(unaccountedCpu / float64(scaler.config.KpNodeCores)))
	}

	if requiredResources.Memory != 0 {
		// Bit shift mebibytes to bytes
		kpNodeMemoryBytes := scaler.config.KpNodeMemory << 20
		// The expected memory resources after in-progress scaling events complete
		expectedMemory := int64(kpNodeMemoryBytes) * int64(numCurrentEvents)
		// The expected amount of memory resources still required after in-progress scaling events complete
		unaccountedMemory := requiredResources.Memory - expectedMemory
		// The least amount of nodes that will satisfy the unaccountedMemory
		numMemoryNodesRequired = int(math.Ceil(float64(unaccountedMemory) / float64(kpNodeMemoryBytes)))
	}

	// The largest of the above two node requirements
	numNodesRequired := int(math.Max(float64(numCpuNodesRequired), float64(numMemoryNodesRequired)))

	for kpNode := 1; kpNode <= numNodesRequired; kpNode++ {
		newName := scaler.newKpNodeName()

		scaleEvent := ScaleEvent{
			ScaleType: 1,
			NodeName:  newName,
		}

		requiredScaleEvents = append(requiredScaleEvents, &scaleEvent)
		logger.DebugLog("Generated scale event", "scaleEvent", fmt.Sprintf("%+v", scaleEvent))
	}

	// If there are no worker nodes then pods can fail to schedule due to a control-plane taint, trigger a scaling event
	if len(requiredScaleEvents) == 0 && numCurrentEvents == 0 {
		schedulingFailed, err := scaler.Kubernetes.IsUnschedulableDueToControlPlaneTaint()
		if err != nil {
			return nil, err
		}

		if schedulingFailed {
			newName := scaler.newKpNodeName()
			scaleEvent := ScaleEvent{
				ScaleType: 1,
				NodeName:  newName,
			}

			requiredScaleEvents = append(requiredScaleEvents, &scaleEvent)
			logger.DebugLog("Generated scale event due to control=plane taint", "scaleEvent", fmt.Sprintf("%+v", scaleEvent))
		}
	}

	return requiredScaleEvents, nil
}

func (scaler *ProxmoxScaler) RequiredScaleEvents(allScaleEvents int) ([]*ScaleEvent, error) {
	unschedulableResources, err := scaler.Kubernetes.GetUnschedulableResources(int64(scaler.config.KpNodeCores), scaler.config.KpNodeNameRegex)
	if err != nil {
		logger.ErrorLog("Failed to get unschedulable resources:", "error", err)
	}

	if unschedulableResources != (kubernetes.UnschedulableResources{}) {
		logger.DebugLog("Found unschedulable resources", "resources", fmt.Sprintf("%+v", unschedulableResources))
	}

	return scaler.requiredScaleEvents(unschedulableResources, allScaleEvents)
}

// func selectTargetHost(hosts []proxmox.HostInformation, kpNodes []proxmox.VmInformation, scaleEvents []*ScaleEvent) proxmox.HostInformation {
// skipHost:
// 	for _, host := range hosts {
// 		// Check for a scaleEvent targeting the pHost
// 		for _, scaleEvent := range scaleEvents {
// 			if scaleEvent.TargetHost.Node == host.Node {
// 				continue skipHost
// 			}
// 		}

// 		for _, kpNode := range kpNodes {
// 			// Check for an existing kpNode on the pHost
// 			if kpNode.Node == host.Node {
// 				continue skipHost
// 			}
// 		}

// 		return host
// 	}

// 	return selectMaxAvailableMemHost(hosts)
// }

// func selectMaxAvailableMemHost(hosts []proxmox.HostInformation) proxmox.HostInformation {
// 	selectedHostHost := hosts[0]
// 	for _, host := range hosts {
// 		if (host.Maxmem - host.Mem) > (selectedHostHost.Maxmem - selectedHostHost.Mem) {
// 			selectedHostHost = host
// 		}
// 	}

// 	return selectedHostHost
// }

func (scaler *ProxmoxScaler) SelectTargetHosts(scaleEvents []*ScaleEvent) error {
	hosts, err := scaler.Proxmox.GetClusterStats()
	if err != nil {
		return err
	}

	kpNodes, err := scaler.Proxmox.GetRunningKpNodes(scaler.config.KpNodeNameRegex)
	if err != nil {
		return err
	}

	// Log the strategy being used
	logger.InfoLog(fmt.Sprintf("Using node selection strategy: %s", scaler.nodeSelector.GetStrategyDescription()))

	for _, scaleEvent := range scaleEvents {
		scaleEvent.TargetHost = scaler.nodeSelector.SelectTargetHost(hosts, kpNodes, scaleEvents)
		logger.DebugLog(fmt.Sprintf("Selected target host %s for %s using %s strategy",
			scaleEvent.TargetHost.Node, scaleEvent.NodeName, scaler.config.NodeSelectionStrategy))
	}

	return nil
}

func (scaler *ProxmoxScaler) renderNodeLabels(scaleEvent *ScaleEvent) (map[string]string, error) {
	labels := map[string]string{}
	for _, label := range strings.Split(scaler.config.KpNodeLabels, ",") {
		key := strings.Split(label, "=")[0]
		value := strings.Split(label, "=")[1]

		templateValues := struct {
			TargetHost string
		}{
			TargetHost: scaleEvent.TargetHost.Node,
		}

		tmpl, err := template.New("labelValue").Parse(value)
		if err != nil {
			logger.WarnLog(fmt.Sprintf("Failed to parse node label template %s=%s, skipping label.", key, value))
			continue
		}

		renderedValue := new(bytes.Buffer)
		err = tmpl.Execute(renderedValue, templateValues)
		if err != nil {
			logger.WarnLog(fmt.Sprintf("Failed to render node label template %s=%s, skipping label.", key, value))
			continue
		}

		labels[key] = renderedValue.String()
	}

	return labels, nil
}

func (scaler *ProxmoxScaler) ScaleUp(ctx context.Context, scaleEvent *ScaleEvent) error {
	logger.InfoLog(fmt.Sprintf("Provisioning %s on %s", scaleEvent.NodeName, scaleEvent.TargetHost.Node))
	pctx, cancelPCtx := context.WithTimeout(
		ctx,
		time.Duration(
			time.Second*time.Duration(
				scaler.config.WaitSecondsForProvision,
			),
		),
	)
	defer cancelPCtx()

	err := scaler.Proxmox.NewKpNode(
		pctx,
		scaleEvent.NodeName,
		scaleEvent.TargetHost.Node,
		scaler.config.KpNodeParams,
		scaler.config.KpLocalTemplateStorage,
		scaler.config.KpNodeTemplateName,
		scaler.config.KpJoinCommand,
	)
	if err != nil {
		return err
	}

	logger.InfoLog(fmt.Sprintf("Started %s", scaleEvent.NodeName))

	if scaler.config.KpQemuExecJoin {
		err = scaler.Proxmox.CheckNodeReady(pctx, scaleEvent.NodeName)
		if err != nil {
			return err
		}

		err = scaler.joinByQemuExec(scaleEvent.NodeName)
		if err != nil {
			return err
		}
	}

	logger.InfoLog(fmt.Sprintf("Waiting for %s to join kubernetes cluster", scaleEvent.NodeName))

	kctx, cancelKCtx := context.WithTimeout(
		ctx,
		time.Duration(
			time.Second*time.Duration(
				scaler.config.WaitSecondsForJoin,
			),
		),
	)
	defer cancelKCtx()

	scaler.Kubernetes.CheckForNodeJoin(
		kctx,
		scaleEvent.NodeName,
	)

	logger.InfoLog(fmt.Sprintf("%s joined kubernetes cluster", scaleEvent.NodeName))

	if scaler.config.KpNodeLabels != "" {
		labels, err := scaler.renderNodeLabels(scaleEvent)
		if err != nil {
			return err
		}

		err = scaler.Kubernetes.LabelKpNode(scaleEvent.NodeName, labels)
		if err != nil {
			return err
		}

		logger.InfoLog(fmt.Sprintf("Set labels on %s", scaleEvent.NodeName))
	}

	return nil
}

func (scaler *ProxmoxScaler) joinByQemuExec(nodeName string) error {
	logger.InfoLog(fmt.Sprintf("Executing join command on %s", nodeName))
	joinExecPid, err := scaler.Proxmox.QemuExecJoin(nodeName, scaler.config.KpJoinCommand)
	if err != nil {
		return err
	}

	var status proxmox.QemuExecStatus

	for {
		status, err = scaler.Proxmox.GetQemuExecJoinStatus(nodeName, joinExecPid)
		if err != nil {
			return err
		}

		if status.Exited == 1 {
			break
		}

		time.Sleep(time.Second * 1)
	}

	if status.ExitCode != 0 {
		return fmt.Errorf("join command for %s failed:\n%s", nodeName, status.OutData)
	} else {
		logger.InfoLog(fmt.Sprintf("Join command for %s executed successfully", nodeName))
		return nil
	}
}

func (scaler *ProxmoxScaler) NumReadyNodes() (int, error) {
	kpNodes, err := scaler.Kubernetes.GetKpNodes(scaler.config.KpNodeNameRegex)
	if err != nil {
		return 0, err
	}

	return len(kpNodes), err
}

func (scaler *ProxmoxScaler) AssessScaleDown() (*ScaleEvent, error) {
	// Check if we should prevent scale-down due to recent scaling activity
	if scaler.shouldPreventScaleDown() {
		logger.DebugLog("Scale-down prevented due to recent scaling activity or insufficient stabilization time")
		return nil, nil
	}

	totalAllocatedResources, err := scaler.GetAllocatedResources()
	if err != nil {
		return nil, fmt.Errorf("failed to get allocated resources: %w", err)
	}

	workerNodesAllocatable, err := scaler.Kubernetes.GetWorkerNodesAllocatableResources()
	if err != nil {
		return nil, fmt.Errorf("failed to get worker nodes capacity: %w", err)
	}

	totalCpuAllocatable := workerNodesAllocatable.Cpu
	totalMemoryAllocatable := workerNodesAllocatable.Memory

	acceptCpuScaleDown := scaler.assessScaleDownForResourceType(totalAllocatedResources.Cpu, totalCpuAllocatable, int64(scaler.config.KpNodeCores))
	acceptMemoryScaleDown := scaler.assessScaleDownForResourceType(totalAllocatedResources.Memory, totalMemoryAllocatable, int64(scaler.config.KpNodeMemory<<20))

	if !(acceptCpuScaleDown && acceptMemoryScaleDown) {
		logger.DebugLog("Scale-down rejected by resource assessment",
			"acceptCpuScaleDown", acceptCpuScaleDown,
			"acceptMemoryScaleDown", acceptMemoryScaleDown,
		)
		return nil, nil
	}

	scaleEvent := ScaleEvent{
		ScaleType: -1,
	}

	err = scaler.selectScaleDownTarget(&scaleEvent)
	if err != nil {
		return nil, err
	}

	// Additional check: ensure the selected node is not too new
	if scaler.isNodeTooNew(scaleEvent.NodeName) {
		logger.DebugLog("Scale-down prevented: selected node is too new", "nodeName", scaleEvent.NodeName)
		return nil, nil
	}

	logger.InfoLog("Scale-down assessment approved", "targetNode", scaleEvent.NodeName)
	return &scaleEvent, nil
}

func (scaler *ProxmoxScaler) assessScaleDownForResourceType(currentResourceAllocated float64, totalResourceAllocatable int64, kpNodeResourceCapacity int64) bool {
	if currentResourceAllocated == 0 {
		return true
	}

	// Prevent division by zero or scaling down below the capacity of a single node
	if totalResourceAllocatable <= kpNodeResourceCapacity {
		logger.DebugLog("AssessScaleDown: Total allocatable is less than or equal to a single node's capacity, preventing scale down.",
			"totalAllocatable", totalResourceAllocatable,
			"kpNodeResourceCapacity", kpNodeResourceCapacity,
		)
		return false
	}

	// Calculate the total allocatable resources AFTER removing one node
	newTotalAllocatable := totalResourceAllocatable - kpNodeResourceCapacity

	// Calculate the projected load percentage on the cluster AFTER removing one node
	// Use float64 for calculation to avoid integer truncation
	projectedLoad := currentResourceAllocated / float64(newTotalAllocatable)

	// Calculate the maximum acceptable load percentage based on headroom
	maxAcceptableLoad := 1.0 - scaler.config.LoadHeadroom

	allowScaleDown := projectedLoad < maxAcceptableLoad

	logger.DebugLog("Assessing scale down for resource type",
		"currentAllocated", currentResourceAllocated,
		"totalAllocatable", totalResourceAllocatable,
		"nodeCapacity", kpNodeResourceCapacity,
		"newTotalAllocatable", newTotalAllocatable,
		"projectedLoad", projectedLoad,
		"maxAcceptableLoad", maxAcceptableLoad,
		"loadHeadroom", scaler.config.LoadHeadroom,
		"allowScaleDown", allowScaleDown,
	)

	// Allow scale down only if the projected load is less than the maximum acceptable load
	return allowScaleDown

	// OLD VERSION
	// The proportion of the cluster's total allocatable resources currently allocated
	// represented as a float between 0 and 1
	// totalResourceLoad := currentResourceAllocated / float64(totalResourceAllocatable)
	// // The expected resource load after scaledown
	// totalCapacityAfterScaleDown := (float64(totalResourceAllocatable-int64(kpNodeResourceCapacity)) / float64(totalResourceAllocatable))
	// acceptableResourceLoadForScaleDown := totalCapacityAfterScaleDown - (totalResourceLoad * scaler.config.LoadHeadroom)

	// return totalResourceLoad < acceptableResourceLoadForScaleDown
}

func (scaler *ProxmoxScaler) selectScaleDownTarget(scaleEvent *ScaleEvent) error {
	if scaleEvent.ScaleType != -1 {
		return fmt.Errorf("expected ScaleEvent ScaleType to be '-1' but got: %d", scaleEvent.ScaleType)
	}

	kpNodes, err := scaler.Kubernetes.GetKpNodes(scaler.config.KpNodeNameRegex)
	if err != nil {
		return err
	}

	if len(kpNodes) == 0 {
		return fmt.Errorf("no nodes to scale down, how did we get here?")
	}

	allocatedResources, err := scaler.Kubernetes.GetKpNodesAllocatedResources(scaler.config.KpNodeNameRegex)
	if err != nil {
		return err
	}

	nodeLoads := make(map[string]float64)

	// Calculate the combined load on each kpNode
	for _, node := range kpNodes {
		nodeLoads[node.Name] =
			(allocatedResources[node.Name].Cpu / float64(scaler.config.KpNodeCores)) +
				(allocatedResources[node.Name].Memory / float64(scaler.config.KpNodeMemory))
	}

	targetNode := kpNodes[0].Name
	// Choose the kpnode with the lowest combined load
	for node := range nodeLoads {
		if nodeLoads[node] < nodeLoads[targetNode] {
			targetNode = node
		}
	}

	scaleEvent.NodeName = targetNode
	return nil
}

func (scaler *ProxmoxScaler) NumNodes() (int, error) {
	nodes, err := scaler.Proxmox.GetAllKpNodes(scaler.config.KpNodeNameRegex)
	return len(nodes), err
}

func (scaler *ProxmoxScaler) ScaleDown(ctx context.Context, scaleEvent *ScaleEvent) error {
	err := scaler.Kubernetes.DeleteKpNode(ctx, scaleEvent.NodeName)
	if err != nil {
		return err
	}

	return scaler.Proxmox.DeleteKpNode(scaleEvent.NodeName, scaler.config.KpNodeNameRegex)
}

// This function is only used when it is unclear whether a node has joined the kubernetes cluster
// ie when cleaning up after a failed scaling event
func (scaler *ProxmoxScaler) DeleteNode(ctx context.Context, kpNodeName string) error {
	_ = scaler.Kubernetes.DeleteKpNode(ctx, kpNodeName)

	return scaler.Proxmox.DeleteKpNode(kpNodeName, scaler.config.KpNodeNameRegex)
}

func (scaler *ProxmoxScaler) GetAllocatableResources() (AllocatableResources, error) {
	var allocatableResources AllocatableResources
	kpNodes, err := scaler.Kubernetes.GetKpNodes(scaler.config.KpNodeNameRegex)
	if err != nil {
		return allocatableResources, err
	}

	for _, kpNode := range kpNodes {
		allocatableResources.Cpu += kpNode.Status.Allocatable.Cpu().AsApproximateFloat64()
		allocatableResources.Memory += kpNode.Status.Allocatable.Memory().AsApproximateFloat64()
	}

	return allocatableResources, nil
}

func (scaler *ProxmoxScaler) GetAllocatedResources() (AllocatedResources, error) {
	// Add a small delay to ensure we get the most up-to-date resource allocation
	// This helps prevent race conditions where pods have been scheduled but
	// the resource calculation hasn't been updated yet
	time.Sleep(2 * time.Second) // This delay might still be relevant or could be re-evaluated

	clusterAllocatedK8s, err := scaler.Kubernetes.GetClusterAllocatedResources()
	if err != nil {
		return AllocatedResources{}, err
	}

	// Convert kubernetes.AllocatedResources to scaler.AllocatedResources
	scalerAllocatedResources := AllocatedResources{
		Cpu:    clusterAllocatedK8s.Cpu,
		Memory: clusterAllocatedK8s.Memory,
	}

	logger.DebugLog("Calculated total cluster allocated resources",
		"totalCpu", fmt.Sprintf("%.2f", scalerAllocatedResources.Cpu),
		"totalMemory", fmt.Sprintf("%.2f", scalerAllocatedResources.Memory),
	)

	return scalerAllocatedResources, nil
}

func (scaler *ProxmoxScaler) GetResourceStatistics() (ResourceStatistics, error) {
	allocatableResources, err := scaler.GetAllocatableResources()
	if err != nil {
		return ResourceStatistics{}, err
	}

	allocatedResources, err := scaler.GetAllocatedResources()
	if err != nil {
		return ResourceStatistics{}, err
	}

	return ResourceStatistics{
		Allocatable: allocatableResources,
		Allocated:   allocatedResources,
	}, nil
}

// shouldPreventScaleDown checks if scale-down should be prevented due to recent scaling activity
func (scaler *ProxmoxScaler) shouldPreventScaleDown() bool {
	// Get all kp nodes to check their creation times
	kpNodes, err := scaler.Kubernetes.GetKpNodes(scaler.config.KpNodeNameRegex)
	if err != nil {
		logger.WarnLog("Failed to get kp nodes for scale-down prevention check", "error", err.Error())
		return true // Err on the side of caution
	}

	// Check if any node was created recently (configurable stabilization period)
	stabilizationPeriod := time.Duration(scaler.config.ScaleDownStabilizationMinutes) * time.Minute
	now := time.Now()

	for _, node := range kpNodes {
		nodeAge := now.Sub(node.CreationTimestamp.Time)
		if nodeAge < stabilizationPeriod {
			logger.DebugLog("Scale-down prevented: recent node found",
				"nodeName", node.Name,
				"nodeAge", nodeAge.String(),
				"stabilizationPeriod", stabilizationPeriod.String(),
			)
			return true
		}
	}

	return false
}

// isNodeTooNew checks if a specific node is too new to be scaled down
func (scaler *ProxmoxScaler) isNodeTooNew(nodeName string) bool {
	kpNodes, err := scaler.Kubernetes.GetKpNodes(scaler.config.KpNodeNameRegex)
	if err != nil {
		logger.WarnLog("Failed to get kp nodes for node age check", "error", err.Error())
		return true // Err on the side of caution
	}

	// Find the specific node
	for _, node := range kpNodes {
		if node.Name == nodeName {
			// Minimum age before a node can be considered for scale-down (configurable)
			minNodeAge := time.Duration(scaler.config.MinNodeAgeMinutes) * time.Minute
			nodeAge := time.Since(node.CreationTimestamp.Time)

			if nodeAge < minNodeAge {
				logger.DebugLog("Node is too new for scale-down",
					"nodeName", nodeName,
					"nodeAge", nodeAge.String(),
					"minNodeAge", minNodeAge.String(),
				)
				return true
			}
			break
		}
	}

	return false
}
