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
	// Check traditional unschedulable resources
	unschedulableResources, err := scaler.Kubernetes.GetUnschedulableResources(int64(scaler.config.KpNodeCores), scaler.config.KpNodeNameRegex)
	if err != nil {
		logger.ErrorLog("Failed to get unschedulable resources:", "error", err)
	}

	if unschedulableResources != (kubernetes.UnschedulableResources{}) {
		logger.DebugLog("Found unschedulable resources", "resources", fmt.Sprintf("%+v", unschedulableResources))
	}

	// Check for enhanced scaling factors
	enhancedScaling, err := scaler.checkEnhancedScalingFactors()
	if err != nil {
		logger.ErrorLog("Failed to check enhanced scaling factors:", "error", err)
	}

	// Combine traditional and enhanced scaling requirements
	combinedResources := scaler.combineScalingRequirements(unschedulableResources, enhancedScaling)

	return scaler.requiredScaleEvents(combinedResources, allScaleEvents)
}

// checkEnhancedScalingFactors evaluates additional scaling triggers beyond unschedulable pods
func (scaler *ProxmoxScaler) checkEnhancedScalingFactors() (kubernetes.UnschedulableResources, error) {
	var enhancedResources kubernetes.UnschedulableResources

	// Check resource pressure thresholds
	if scaler.config.EnableResourcePressureScaling {
		resourcePressure, err := scaler.checkResourcePressure()
		if err != nil {
			logger.WarnLog("Failed to check resource pressure", "error", err)
		} else {
			enhancedResources.Cpu += resourcePressure.Cpu
			enhancedResources.Memory += resourcePressure.Memory
		}
	}

	// Check for scheduling errors
	if scaler.config.EnableSchedulingErrorScaling {
		schedulingPressure, err := scaler.checkSchedulingErrors()
		if err != nil {
			logger.WarnLog("Failed to check scheduling errors", "error", err)
		} else {
			enhancedResources.Cpu += schedulingPressure.Cpu
			enhancedResources.Memory += schedulingPressure.Memory
		}
	}

	// Check storage pressure
	if scaler.config.EnableStoragePressureScaling {
		storagePressure, err := scaler.checkStoragePressure()
		if err != nil {
			logger.WarnLog("Failed to check storage pressure", "error", err)
		} else {
			enhancedResources.Cpu += storagePressure.Cpu
			enhancedResources.Memory += storagePressure.Memory
		}
	}

	return enhancedResources, nil
}

// combineScalingRequirements merges traditional and enhanced scaling requirements
func (scaler *ProxmoxScaler) combineScalingRequirements(traditional, enhanced kubernetes.UnschedulableResources) kubernetes.UnschedulableResources {
	return kubernetes.UnschedulableResources{
		Cpu:    math.Max(traditional.Cpu, enhanced.Cpu),
		Memory: traditional.Memory + enhanced.Memory, // Memory requirements are additive
	}
}

// checkResourcePressure evaluates CPU and memory utilization thresholds
func (scaler *ProxmoxScaler) checkResourcePressure() (kubernetes.UnschedulableResources, error) {
	var pressureResources kubernetes.UnschedulableResources

	utilization, err := scaler.Kubernetes.GetClusterResourceUtilization()
	if err != nil {
		return pressureResources, fmt.Errorf("failed to get cluster resource utilization: %w", err)
	}

	logger.DebugLog("Current cluster resource utilization",
		"cpuUtilization", utilization.CpuUtilization,
		"memoryUtilization", utilization.MemoryUtilization)

	// Check CPU pressure
	if utilization.CpuUtilization > scaler.config.CpuUtilizationThreshold {
		// Request additional CPU capacity equivalent to one node
		pressureResources.Cpu = float64(scaler.config.KpNodeCores)
		logger.InfoLog("CPU utilization threshold exceeded, triggering scale-up",
			"currentUtilization", utilization.CpuUtilization,
			"threshold", scaler.config.CpuUtilizationThreshold)
	}

	// Check memory pressure
	if utilization.MemoryUtilization > scaler.config.MemoryUtilizationThreshold {
		// Request additional memory capacity equivalent to one node
		pressureResources.Memory = int64(scaler.config.KpNodeMemory << 20) // Convert MB to bytes
		logger.InfoLog("Memory utilization threshold exceeded, triggering scale-up",
			"currentUtilization", utilization.MemoryUtilization,
			"threshold", scaler.config.MemoryUtilizationThreshold)
	}

	return pressureResources, nil
}

// checkSchedulingErrors evaluates recent pod scheduling failures
func (scaler *ProxmoxScaler) checkSchedulingErrors() (kubernetes.UnschedulableResources, error) {
	var errorResources kubernetes.UnschedulableResources

	// Look for scheduling errors in the last 5 minutes
	timeWindow := 5 * time.Minute
	schedulingErrors, err := scaler.Kubernetes.GetRecentSchedulingErrors(timeWindow)
	if err != nil {
		return errorResources, fmt.Errorf("failed to get recent scheduling errors: %w", err)
	}

	// Count unique pods with scheduling errors
	uniqueFailedPods := make(map[string]bool)
	totalFailures := 0
	for _, error := range schedulingErrors {
		podKey := fmt.Sprintf("%s/%s", error.Namespace, error.PodName)
		uniqueFailedPods[podKey] = true
		totalFailures += error.FailureCount
	}

	logger.DebugLog("Scheduling error analysis",
		"uniqueFailedPods", len(uniqueFailedPods),
		"totalFailures", totalFailures,
		"threshold", scaler.config.SchedulingErrorThreshold)

	// Trigger scaling if we have enough scheduling errors
	if len(uniqueFailedPods) >= scaler.config.SchedulingErrorThreshold {
		// Request capacity for one additional node
		errorResources.Cpu = float64(scaler.config.KpNodeCores)
		errorResources.Memory = int64(scaler.config.KpNodeMemory << 20)
		logger.InfoLog("Scheduling error threshold exceeded, triggering scale-up",
			"failedPods", len(uniqueFailedPods),
			"threshold", scaler.config.SchedulingErrorThreshold)
	}

	return errorResources, nil
}

// checkStoragePressure evaluates disk space utilization across nodes
func (scaler *ProxmoxScaler) checkStoragePressure() (kubernetes.UnschedulableResources, error) {
	var storageResources kubernetes.UnschedulableResources

	diskUtilization, err := scaler.Kubernetes.GetNodeDiskUtilization()
	if err != nil {
		return storageResources, fmt.Errorf("failed to get node disk utilization: %w", err)
	}

	// Check if any node exceeds storage thresholds
	for nodeName, disk := range diskUtilization {
		logger.DebugLog("Node disk utilization",
			"node", nodeName,
			"utilizationPercent", disk.UtilizationPercent,
			"availableGB", disk.AvailableDiskSpaceGB)

		// Check disk utilization percentage threshold
		if disk.UtilizationPercent > scaler.config.DiskUtilizationThreshold {
			storageResources.Cpu = float64(scaler.config.KpNodeCores)
			storageResources.Memory = int64(scaler.config.KpNodeMemory << 20)
			logger.InfoLog("Disk utilization threshold exceeded, triggering scale-up",
				"node", nodeName,
				"currentUtilization", disk.UtilizationPercent,
				"threshold", scaler.config.DiskUtilizationThreshold)
			break
		}

		// Check minimum available disk space threshold
		if disk.AvailableDiskSpaceGB < float64(scaler.config.MinAvailableDiskSpaceGB) {
			storageResources.Cpu = float64(scaler.config.KpNodeCores)
			storageResources.Memory = int64(scaler.config.KpNodeMemory << 20)
			logger.InfoLog("Minimum disk space threshold exceeded, triggering scale-up",
				"node", nodeName,
				"availableGB", disk.AvailableDiskSpaceGB,
				"minimumGB", scaler.config.MinAvailableDiskSpaceGB)
			break
		}
	}

	return storageResources, nil
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

	// Check enhanced scaling factors to prevent scale-down during pressure
	if scaler.shouldPreventScaleDownDueToEnhancedFactors() {
		logger.DebugLog("Scale-down prevented due to enhanced scaling factors indicating pressure")
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

	// Final check: ensure removing this specific node won't cause enhanced scaling pressure
	if scaler.wouldScaleDownCauseEnhancedPressure(scaleEvent.NodeName) {
		logger.DebugLog("Scale-down prevented: removing node would cause enhanced scaling pressure", "nodeName", scaleEvent.NodeName)
		return nil, nil
	}

	logger.InfoLog("Scale-down assessment approved", "targetNode", scaleEvent.NodeName)
	return &scaleEvent, nil
}

func (scaler *ProxmoxScaler) assessScaleDownForResourceType(currentResourceAllocated float64, totalResourceAllocatable int64, kpNodeResourceCapacity int64) bool {
	if currentResourceAllocated == 0 {
		return true
	}

	// The proportion of the cluster's total allocatable resources currently allocated
	// represented as a float between 0 and 1
	totalResourceLoad := currentResourceAllocated / float64(totalResourceAllocatable)
	// The expected resource load after scaledown
	totalCapacityAfterScaleDown := (float64(totalResourceAllocatable-int64(kpNodeResourceCapacity)) / float64(totalResourceAllocatable))
	acceptableResourceLoadForScaleDown := totalCapacityAfterScaleDown - (totalResourceLoad * scaler.config.LoadHeadroom)

	return totalResourceLoad < acceptableResourceLoadForScaleDown
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
	time.Sleep(2 * time.Second)

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

// shouldPreventScaleDownDueToEnhancedFactors checks if enhanced scaling factors indicate pressure
// that would prevent safe scale-down
func (scaler *ProxmoxScaler) shouldPreventScaleDownDueToEnhancedFactors() bool {
	// Check resource pressure thresholds with scale-down safety margins
	if scaler.config.EnableResourcePressureScaling {
		if scaler.isResourcePressurePreventingScaleDown() {
			return true
		}
	}

	// Check for recent scheduling errors
	if scaler.config.EnableSchedulingErrorScaling {
		if scaler.areSchedulingErrorsPreventingScaleDown() {
			return true
		}
	}

	// Check storage pressure
	if scaler.config.EnableStoragePressureScaling {
		if scaler.isStoragePressurePreventingScaleDown() {
			return true
		}
	}

	return false
}

// wouldScaleDownCauseEnhancedPressure checks if removing a specific node would cause
// enhanced scaling factors to trigger, indicating the scale-down should be prevented
func (scaler *ProxmoxScaler) wouldScaleDownCauseEnhancedPressure(nodeName string) bool {
	// For now, we rely on the general enhanced factors check
	// In the future, this could simulate removing the specific node and check
	// if that would cause resource pressure to exceed thresholds

	// This is a placeholder for more sophisticated logic that could:
	// 1. Calculate what resource utilization would be after removing this node
	// 2. Check if that would exceed our thresholds
	// 3. Verify that remaining nodes can handle the workload

	logger.DebugLog("Enhanced pressure check for specific node scale-down", "nodeName", nodeName)
	return false // For now, allow the general checks to handle this
}

// isResourcePressurePreventingScaleDown checks if current resource utilization is too high
// to safely scale down. Uses more conservative thresholds than scale-up to provide safety margin.
func (scaler *ProxmoxScaler) isResourcePressurePreventingScaleDown() bool {
	utilization, err := scaler.Kubernetes.GetClusterResourceUtilization()
	if err != nil {
		logger.WarnLog("Failed to get cluster resource utilization for scale-down check", "error", err)
		return true // Err on the side of caution
	}

	// Use more conservative thresholds for scale-down prevention (10% lower than scale-up thresholds)
	cpuScaleDownThreshold := scaler.config.CpuUtilizationThreshold - 0.1
	memoryScaleDownThreshold := scaler.config.MemoryUtilizationThreshold - 0.1

	// Ensure thresholds don't go below reasonable minimums
	if cpuScaleDownThreshold < 0.5 {
		cpuScaleDownThreshold = 0.5
	}
	if memoryScaleDownThreshold < 0.5 {
		memoryScaleDownThreshold = 0.5
	}

	logger.DebugLog("Resource pressure scale-down check",
		"cpuUtilization", utilization.CpuUtilization,
		"memoryUtilization", utilization.MemoryUtilization,
		"cpuScaleDownThreshold", cpuScaleDownThreshold,
		"memoryScaleDownThreshold", memoryScaleDownThreshold)

	// Prevent scale-down if either CPU or memory utilization is too high
	if utilization.CpuUtilization > cpuScaleDownThreshold {
		logger.InfoLog("Scale-down prevented due to high CPU utilization",
			"currentUtilization", utilization.CpuUtilization,
			"scaleDownThreshold", cpuScaleDownThreshold)
		return true
	}

	if utilization.MemoryUtilization > memoryScaleDownThreshold {
		logger.InfoLog("Scale-down prevented due to high memory utilization",
			"currentUtilization", utilization.MemoryUtilization,
			"scaleDownThreshold", memoryScaleDownThreshold)
		return true
	}

	return false
}

// areSchedulingErrorsPreventingScaleDown checks if there are recent scheduling errors
// that indicate the cluster is under pressure and shouldn't be scaled down
func (scaler *ProxmoxScaler) areSchedulingErrorsPreventingScaleDown() bool {
	// Look for scheduling errors in the last 10 minutes (longer window for scale-down)
	timeWindow := 10 * time.Minute
	schedulingErrors, err := scaler.Kubernetes.GetRecentSchedulingErrors(timeWindow)
	if err != nil {
		logger.WarnLog("Failed to get recent scheduling errors for scale-down check", "error", err)
		return true // Err on the side of caution
	}

	// Count unique pods with scheduling errors
	uniqueFailedPods := make(map[string]bool)
	for _, error := range schedulingErrors {
		podKey := fmt.Sprintf("%s/%s", error.Namespace, error.PodName)
		uniqueFailedPods[podKey] = true
	}

	// Use a lower threshold for scale-down prevention (half of scale-up threshold)
	scaleDownErrorThreshold := scaler.config.SchedulingErrorThreshold / 2
	if scaleDownErrorThreshold < 1 {
		scaleDownErrorThreshold = 1
	}

	logger.DebugLog("Scheduling error scale-down check",
		"uniqueFailedPods", len(uniqueFailedPods),
		"scaleDownErrorThreshold", scaleDownErrorThreshold,
		"timeWindow", timeWindow.String())

	if len(uniqueFailedPods) >= scaleDownErrorThreshold {
		logger.InfoLog("Scale-down prevented due to recent scheduling errors",
			"failedPods", len(uniqueFailedPods),
			"scaleDownThreshold", scaleDownErrorThreshold)
		return true
	}

	return false
}

// isStoragePressurePreventingScaleDown checks if any nodes have storage pressure
// that would make scale-down unsafe
func (scaler *ProxmoxScaler) isStoragePressurePreventingScaleDown() bool {
	diskUtilization, err := scaler.Kubernetes.GetNodeDiskUtilization()
	if err != nil {
		logger.WarnLog("Failed to get node disk utilization for scale-down check", "error", err)
		return true // Err on the side of caution
	}

	// Use more conservative thresholds for scale-down prevention (5% lower than scale-up thresholds)
	diskScaleDownThreshold := scaler.config.DiskUtilizationThreshold - 0.05
	minDiskScaleDownGB := float64(scaler.config.MinAvailableDiskSpaceGB) + 2.0 // 2GB extra buffer

	// Ensure thresholds don't go below reasonable minimums
	if diskScaleDownThreshold < 0.7 {
		diskScaleDownThreshold = 0.7
	}

	// Check if any node exceeds storage thresholds
	for nodeName, disk := range diskUtilization {
		logger.DebugLog("Node disk utilization scale-down check",
			"node", nodeName,
			"utilizationPercent", disk.UtilizationPercent,
			"availableGB", disk.AvailableDiskSpaceGB,
			"scaleDownThreshold", diskScaleDownThreshold,
			"minDiskScaleDownGB", minDiskScaleDownGB)

		// Check disk utilization percentage threshold
		if disk.UtilizationPercent > diskScaleDownThreshold {
			logger.InfoLog("Scale-down prevented due to high disk utilization",
				"node", nodeName,
				"currentUtilization", disk.UtilizationPercent,
				"scaleDownThreshold", diskScaleDownThreshold)
			return true
		}

		// Check minimum available disk space threshold
		if disk.AvailableDiskSpaceGB < minDiskScaleDownGB {
			logger.InfoLog("Scale-down prevented due to low available disk space",
				"node", nodeName,
				"availableGB", disk.AvailableDiskSpaceGB,
				"minRequiredGB", minDiskScaleDownGB)
			return true
		}
	}

	return false
}
