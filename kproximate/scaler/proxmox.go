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

	"github.com/jedrw/kproximate/config"
	"github.com/jedrw/kproximate/kubernetes"
	"github.com/jedrw/kproximate/logger"
	"github.com/jedrw/kproximate/proxmox"
	"k8s.io/apimachinery/pkg/util/uuid"
)

type ProxmoxScaler struct {
	config     config.KproximateConfig
	Kubernetes kubernetes.Kubernetes
	Proxmox    proxmox.Proxmox
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
		config:     config,
		Kubernetes: &kubernetes,
		Proxmox:    &proxmox,
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

func selectTargetHost(hosts []proxmox.HostInformation, kpNodes []proxmox.VmInformation, scaleEvents []*ScaleEvent) proxmox.HostInformation {
skipHost:
	for _, host := range hosts {
		// Check for a scaleEvent targeting the pHost
		for _, scaleEvent := range scaleEvents {
			if scaleEvent.TargetHost.Node == host.Node {
				continue skipHost
			}
		}

		for _, kpNode := range kpNodes {
			// Check for an existing kpNode on the pHost
			if kpNode.Node == host.Node {
				continue skipHost
			}
		}

		return host
	}

	return selectMaxAvailableMemHost(hosts)
}

func selectMaxAvailableMemHost(hosts []proxmox.HostInformation) proxmox.HostInformation {
	selectedHostHost := hosts[0]
	for _, host := range hosts {
		if (host.Maxmem - host.Mem) > (selectedHostHost.Maxmem - selectedHostHost.Mem) {
			selectedHostHost = host
		}
	}

	return selectedHostHost
}

func (scaler *ProxmoxScaler) SelectTargetHosts(scaleEvents []*ScaleEvent) error {
	hosts, err := scaler.Proxmox.GetClusterStats()
	if err != nil {
		return err
	}

	kpNodes, err := scaler.Proxmox.GetRunningKpNodes(scaler.config.KpNodeNameRegex)
	if err != nil {
		return err
	}

	for _, scaleEvent := range scaleEvents {
		scaleEvent.TargetHost = selectTargetHost(hosts, kpNodes, scaleEvents)
		logger.DebugLog(fmt.Sprintf("Selected target host %s for %s", scaleEvent.TargetHost.Node, scaleEvent.NodeName))
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
		return nil, nil
	}

	scaleEvent := ScaleEvent{
		ScaleType: -1,
	}

	err = scaler.selectScaleDownTarget(&scaleEvent)
	if err != nil {
		return nil, err
	}

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
	var allocatedResources AllocatedResources
	resources, err := scaler.Kubernetes.GetKpNodesAllocatedResources(scaler.config.KpNodeNameRegex)
	if err != nil {
		return allocatedResources, err
	}

	for _, kpNode := range resources {
		allocatedResources.Cpu += kpNode.Cpu
		allocatedResources.Memory += kpNode.Memory
	}

	return allocatedResources, nil
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
