package scaler

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/paradoxe35/kproximate/config"
	"github.com/paradoxe35/kproximate/kubernetes"
	"github.com/paradoxe35/kproximate/proxmox"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
)

func TestRequiredScaleEventsFor1CPU(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			UnschedulableResources: kubernetes.UnschedulableResources{
				Cpu:    1.0,
				Memory: 0,
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:  2,
			KpNodeMemory: 2048,
			MaxKpNodes:   3,
		},
	}

	currentEvents := 0

	requiredScaleEvents, err := s.RequiredScaleEvents(currentEvents)
	if err != nil {
		t.Error(err)
	}

	if len(requiredScaleEvents) != 1 {
		t.Errorf("Expected exactly 1 scaleEvent, got: %d", len(requiredScaleEvents))
	}
}

func TestRequiredScaleEventsFor3CPU(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			UnschedulableResources: kubernetes.UnschedulableResources{
				Cpu:    3.0,
				Memory: 0,
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:  2,
			KpNodeMemory: 2048,
			MaxKpNodes:   3,
		},
	}

	currentEvents := 0

	requiredScaleEvents, err := s.RequiredScaleEvents(currentEvents)
	if err != nil {
		t.Error(err)
	}

	if len(requiredScaleEvents) != 2 {
		t.Errorf("Expected exactly 2 scaleEvents, got: %d", len(requiredScaleEvents))
	}
}

func TestRequiredScaleEventsFor1024MBMemory(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			UnschedulableResources: kubernetes.UnschedulableResources{
				Cpu:    0,
				Memory: 1073741824,
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:  2,
			KpNodeMemory: 2048,
			MaxKpNodes:   3,
		},
	}

	currentEvents := 0

	requiredScaleEvents, err := s.RequiredScaleEvents(currentEvents)
	if err != nil {
		t.Error(err)
	}

	if len(requiredScaleEvents) != 1 {
		t.Errorf("Expected exactly 1 scaleEvent, got: %d", len(requiredScaleEvents))
	}
}

func TestRequiredScaleEventsFor3072MBMemory(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			UnschedulableResources: kubernetes.UnschedulableResources{
				Cpu:    0,
				Memory: 3221225472,
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:  2,
			KpNodeMemory: 2048,
			MaxKpNodes:   3,
		},
	}

	currentEvents := 0

	requiredScaleEvents, err := s.RequiredScaleEvents(currentEvents)
	if err != nil {
		t.Error(err)
	}

	if len(requiredScaleEvents) != 2 {
		t.Errorf("Expected exactly 2 scaleEvent, got: %d", len(requiredScaleEvents))
	}
}

func TestRequiredScaleEventsFor1CPU3072MBMemory(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			UnschedulableResources: kubernetes.UnschedulableResources{
				Cpu:    1,
				Memory: 3221225472,
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:  2,
			KpNodeMemory: 2048,
			MaxKpNodes:   3,
		},
	}

	currentEvents := 0

	requiredScaleEvents, err := s.RequiredScaleEvents(currentEvents)
	if err != nil {
		t.Error(err)
	}

	if len(requiredScaleEvents) != 2 {
		t.Errorf("Expected exactly 2 scaleEvent, got: %d", len(requiredScaleEvents))
	}
}

func TestRequiredScaleEventsFor4CPU4096MBMemory(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			UnschedulableResources: kubernetes.UnschedulableResources{
				Cpu:    4,
				Memory: 4294967296,
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:  4,
			KpNodeMemory: 4096,
			MaxKpNodes:   4,
		},
	}

	currentEvents := 0

	requiredScaleEvents, err := s.RequiredScaleEvents(currentEvents)
	if err != nil {
		t.Error(err)
	}

	if len(requiredScaleEvents) != 1 {
		t.Errorf("Expected exactly 1 scaleEvent, got: %d", len(requiredScaleEvents))
	}
}

func TestRequiredScaleEventsFor1CPU3072MBMemory1QueuedEvent(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			UnschedulableResources: kubernetes.UnschedulableResources{
				Cpu:    1,
				Memory: 3221225472,
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:  2,
			KpNodeMemory: 2048,
			MaxKpNodes:   3,
		},
	}

	currentEvents := 1

	requiredScaleEvents, err := s.RequiredScaleEvents(currentEvents)
	if err != nil {
		t.Error(err)
	}

	if len(requiredScaleEvents) != 1 {
		t.Errorf("Expected exactly 1 scaleEvent, got: %d", len(requiredScaleEvents))
	}
}

func TestSelectTargetHosts(t *testing.T) {
	cfg := config.KproximateConfig{
		KpNodeNameRegex:       *regexp.MustCompile(`^kp-node-\w{8}-\w{4}-\w{4}-\w{4}-\w{12}$`),
		KpNodeNamePrefix:      "kp-node",
		NodeSelectionStrategy: "spread",
		MinAvailableCpuCores:  0,
		MinAvailableMemoryMB:  0,
		ExcludedNodes:         "",
	}

	s := ProxmoxScaler{
		Proxmox: &proxmox.ProxmoxMock{
			ClusterStats: []proxmox.HostInformation{
				{
					Id:     "node/host-01",
					Node:   "host-01",
					Cpu:    0.209377325725626,
					Mem:    20394792448,
					Maxmem: 16647962624,
					Maxcpu: 48,
					Status: "online",
				},
				{
					Id:     "node/host-02",
					Node:   "host-02",
					Cpu:    0.209377325725626,
					Mem:    20394792448,
					Maxmem: 16647962624,
					Maxcpu: 40,
					Status: "online",
				},
				{
					Id:     "node/host-03",
					Node:   "host-03",
					Cpu:    0.209377325725626,
					Mem:    11394792448,
					Maxmem: 16647962624,
					Maxcpu: 16,
					Status: "online",
				},
			},
			RunningKpNodes: []proxmox.VmInformation{
				{
					Cpu:     0.114889359119222,
					MaxDisk: 10737418240,
					MaxMem:  2147483648,
					Mem:     1074127542,
					Name:    "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd",
					NetIn:   35838253204,
					NetOut:  56111331754,
					Node:    "host-03",
					Status:  "running",
					Uptime:  740227,
					VmID:    603,
				},
			},
		},
		config:       cfg,
		nodeSelector: NewNodeSelector(cfg),
	}

	scaleEvents := []*ScaleEvent{
		{
			ScaleType: 1,
			NodeName:  fmt.Sprintf("%s-%s", s.config.KpNodeNamePrefix, uuid.NewUUID()),
		},
		{
			ScaleType: 1,
			NodeName:  fmt.Sprintf("%s-%s", s.config.KpNodeNamePrefix, uuid.NewUUID()),
		},
		{
			ScaleType: 1,
			NodeName:  fmt.Sprintf("%s-%s", s.config.KpNodeNamePrefix, uuid.NewUUID()),
		},
	}

	err := s.SelectTargetHosts(scaleEvents)
	if err != nil {
		t.Error(err)
	}

	// With spread strategy, the selection should work as follows:
	// 1. All hosts are eligible (no resource restrictions in this test)
	// 2. The spread strategy should prefer hosts without existing kpNodes
	// 3. Since host-03 has an existing kpNode, it should be less preferred

	// Count how many times each host was selected
	hostCounts := make(map[string]int)
	for _, event := range scaleEvents {
		hostCounts[event.TargetHost.Node]++
	}

	// Verify that all scale events got assigned to hosts
	totalAssignments := hostCounts["host-01"] + hostCounts["host-02"] + hostCounts["host-03"]
	if totalAssignments != 3 {
		t.Errorf("Expected 3 total host assignments, got %d", totalAssignments)
	}

	// With the spread strategy, we expect host-01 and host-02 to be preferred over host-03
	// since host-03 already has a kpNode
	if hostCounts["host-01"] == 0 && hostCounts["host-02"] == 0 {
		t.Error("Expected at least one assignment to host-01 or host-02 (hosts without existing kpNodes)")
	}
}

func TestAssessScaleDownForResourceTypeZeroLoad(t *testing.T) {
	scaler := ProxmoxScaler{
		config: config.KproximateConfig{
			LoadHeadroom: 0.2,
		},
	}

	scaleDownZeroLoad := scaler.assessScaleDownForResourceType(0, 5, 5)
	if !scaleDownZeroLoad {
		t.Errorf("Expected true but got %t", scaleDownZeroLoad)
	}
}

func TestAssessScaleDownForResourceTypeAcceptable(t *testing.T) {
	scaler := ProxmoxScaler{
		config: config.KproximateConfig{
			LoadHeadroom: 0.2,
		},
	}

	scaleDownAcceptable := scaler.assessScaleDownForResourceType(5, 10, 2)
	if !scaleDownAcceptable {
		t.Errorf("Expected true but got %t", scaleDownAcceptable)
	}
}

func TestAssessScaleDownForResourceTypeUnAcceptable(t *testing.T) {
	scaler := ProxmoxScaler{
		config: config.KproximateConfig{
			LoadHeadroom: 0.2,
		},
	}

	scaleDownUnAcceptable := scaler.assessScaleDownForResourceType(7, 10, 2)
	if scaleDownUnAcceptable {
		t.Errorf("Expected false but got %t", scaleDownUnAcceptable)
	}
}

func TestSelectScaleDownTarget(t *testing.T) {
	node1 := apiv1.Node{}
	node1.Name = "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd"
	node2 := apiv1.Node{}
	node2.Name = "kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a"
	node3 := apiv1.Node{}
	node3.Name = "kp-node-67944692-1de7-4bd0-ac8c-de6dc178cb38"

	scaler := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			KpNodes: []apiv1.Node{
				node1,
				node2,
				node3,
			},
			AllocatedResources: map[string]kubernetes.AllocatedResources{
				"kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd": {
					Cpu:    1.0,
					Memory: 2048.0,
				},
				"kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a": {
					Cpu:    1.0,
					Memory: 2048.0,
				},
				"kp-node-67944692-1de7-4bd0-ac8c-de6dc178cb38": {
					Cpu:    1.0,
					Memory: 1048.0,
				},
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:  2,
			KpNodeMemory: 1024,
		},
	}

	scaleEvent := ScaleEvent{
		ScaleType: -1,
	}

	scaler.selectScaleDownTarget(&scaleEvent)

	if scaleEvent.NodeName != "kp-node-67944692-1de7-4bd0-ac8c-de6dc178cb38" {
		t.Errorf("Expected kp-node-67944692-1de7-4bd0-ac8c-de6dc178cb38 but got %s", scaleEvent.NodeName)
	}
}

func TestAssessScaleDownIsAcceptable(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			AllocatedResources: map[string]kubernetes.AllocatedResources{
				"kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd": {
					Cpu:    1.0,
					Memory: 1073741824.0,
				},
				"kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a": {
					Cpu:    1.0,
					Memory: 1073741824.0,
				},
				"kp-node-67944692-1de7-4bd0-ac8c-de6dc178cb38": {
					Cpu:    1.0,
					Memory: 1073741824.0,
				},
			},
			WorkerNodesAllocatableResources: kubernetes.WorkerNodesAllocatableResources{
				Cpu:    6,
				Memory: 6442450944,
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:  2,
			KpNodeMemory: 2048,
			LoadHeadroom: 0.2,
		},
	}

	scaleEvent, err := s.AssessScaleDown()
	if err != nil {
		t.Error(err)
	}

	if scaleEvent == nil {
		t.Error("AssessScaleDown returned nil")
	} else if scaleEvent.NodeName == "" {
		t.Error("scaleEvent had no NodeName")
	}

}

func TestAssessScaleDownIsUnacceptable(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			// AllocatedResources is no longer directly used by GetAllocatedResources,
			// but KpNodes might be derived from it if not explicitly set.
			// We now need to set MockClusterAllocatedResources.
			AllocatedResources: map[string]kubernetes.AllocatedResources{
				"kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd": {
					Cpu:    2.0,
					Memory: 2147483648.0,
				},
				"kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a": {
					Cpu:    2.0,
					Memory: 2147483648.0,
				},
				"kp-node-67944692-1de7-4bd0-ac8c-de6dc178cb38": {
					Cpu:    2.0,
					Memory: 2147483648.0,
				},
				"kp-node-a3c5e4ef-4713-473f-b9f7-3abe413c38ff": {
					Cpu:    2.0,
					Memory: 2147483648.0,
				},
				"kp-node-97d74769-22af-420d-9f5e-b2d3c7dd6e7e": {
					Cpu:    1.0,
					Memory: 1073741824.0,
				},
				"kp-node-96f665dd-21c3-4ce1-a1e4-c7717c5338a3": {
					Cpu:    1.0,
					Memory: 1073741824.0,
				},
				"kp-node-76f665dd-21c3-4ce1-a1e4-c7717c5338a3": {
					Cpu:    0.0,
					Memory: 0.0,
				},
			},
			WorkerNodesAllocatableResources: kubernetes.WorkerNodesAllocatableResources{
				Cpu:    14,
				Memory: 15032385536, // 14336 MiB
			},
			// Set the cluster-wide allocated resources to what the sum of kp-node resources was
			MockClusterAllocatedResources: kubernetes.AllocatedResources{
				Cpu:    10.0,          // Sum of CPU from original AllocatedResources map
				Memory: 10737418240.0, // Sum of Memory (10240 MiB)
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:  2,
			KpNodeMemory: 2048,
			LoadHeadroom: 0.2,
		},
	}

	scaleEvent, err := s.AssessScaleDown()
	if err != nil {
		t.Error(err)
	}

	if scaleEvent != nil {
		t.Error("AssessScaleDown did not return nil")
	}
}

func TestScaleDownStabilizationPrevention(t *testing.T) {
	// Create a mock node that was created recently
	recentNode := apiv1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "kp-node-recent",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Minute)), // 2 minutes ago
		},
	}

	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			KpNodes: []apiv1.Node{recentNode},
			WorkerNodesAllocatableResources: kubernetes.WorkerNodesAllocatableResources{
				Cpu:    4,
				Memory: 4294967296, // 4GB
			},
			AllocatedResources: map[string]kubernetes.AllocatedResources{
				"kp-node-recent": {
					Cpu:    1.0,
					Memory: 1073741824.0, // 1GB
				},
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:                   2,
			KpNodeMemory:                  2048,
			LoadHeadroom:                  0.2,
			ScaleDownStabilizationMinutes: 5,  // 5 minutes stabilization
			MinNodeAgeMinutes:             10, // 10 minutes minimum age
		},
	}

	scaleEvent, err := s.AssessScaleDown()
	if err != nil {
		t.Error(err)
	}

	// Should return nil because of recent node creation
	if scaleEvent != nil {
		t.Error("Expected scale-down to be prevented due to recent node creation")
	}
}

func TestScaleDownNodeTooNewPrevention(t *testing.T) {
	// Create a mock node that is old enough for stabilization but too new for individual scale-down
	oldEnoughForStabilization := apiv1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "kp-node-old-enough",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-20 * time.Minute)), // 20 minutes ago
		},
	}

	newNode := apiv1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "kp-node-new",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)), // 5 minutes ago
		},
	}

	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			KpNodes: []apiv1.Node{oldEnoughForStabilization, newNode},
			WorkerNodesAllocatableResources: kubernetes.WorkerNodesAllocatableResources{
				Cpu:    8,
				Memory: 8589934592, // 8GB
			},
			AllocatedResources: map[string]kubernetes.AllocatedResources{
				"kp-node-old-enough": {
					Cpu:    1.5,          // Higher load to make it less preferred
					Memory: 1073741824.0, // 1GB
				},
				"kp-node-new": {
					Cpu:    0.1,         // Lower load to make it more preferred for scale-down
					Memory: 107374182.0, // 0.1GB
				},
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:                   2,
			KpNodeMemory:                  2048,
			LoadHeadroom:                  0.2,
			ScaleDownStabilizationMinutes: 3,  // 3 minutes stabilization (passed)
			MinNodeAgeMinutes:             10, // 10 minutes minimum age
		},
	}

	scaleEvent, err := s.AssessScaleDown()
	if err != nil {
		t.Error(err)
	}

	// Should return nil because the selected node (lowest load) would be too new
	if scaleEvent != nil {
		t.Errorf("Expected scale-down to be prevented due to selected node being too new, but got scale event for: %s", scaleEvent.NodeName)
	}
}

func TestJoinByQemuExecSuccess(t *testing.T) {
	s := ProxmoxScaler{
		Proxmox: &proxmox.ProxmoxMock{
			JoinExecPid: 1,
			QemuExecJoinStatus: proxmox.QemuExecStatus{
				Exited:   1,
				ExitCode: 0,
				OutData:  "We shouldnt see this!",
			},
		},
		config: config.KproximateConfig{
			KpJoinCommand: "echo test",
		},
	}

	kpNodeName := "kp-node-96f665dd-21c3-4ce1-a1e4-c7717c5338a3"

	err := s.joinByQemuExec(kpNodeName)

	if err != nil {
		t.Errorf("Expected nil, Got %s", err)
	}
}

func TestJoinByQemuExecFail(t *testing.T) {
	s := ProxmoxScaler{
		Proxmox: &proxmox.ProxmoxMock{
			JoinExecPid: 1,
			QemuExecJoinStatus: proxmox.QemuExecStatus{
				Exited:   1,
				ExitCode: 1,
				OutData:  "The join command failed!",
			},
		},
		config: config.KproximateConfig{
			KpJoinCommand: "echo test",
		},
	}

	kpNodeName := "kp-node-96f665dd-21c3-4ce1-a1e4-c7717c5338a3"

	err := s.joinByQemuExec(kpNodeName)

	if err == nil {
		t.Error("Expected the join command to fail")
	}
}

func TestParseNodeLabels(t *testing.T) {
	s := ProxmoxScaler{
		config: config.KproximateConfig{
			KpNodeLabels: "topology.kubernetes.io/region=proxmox-cluster,topology.kubernetes.io/zone={{ .TargetHost }}",
		},
	}

	labels, err := s.renderNodeLabels(
		&ScaleEvent{
			TargetHost: proxmox.HostInformation{
				Node: "proxmox-node-01",
			},
		},
	)

	if err != nil {
		t.Error(err)
	}

	if labels["topology.kubernetes.io/region"] != "proxmox-cluster" {
		t.Errorf("Expected topology.kubernetes.io/region label to have 'proxmox-cluster' as value, got %s", labels["topology.kubernetes.io/region"])
	}

	if labels["topology.kubernetes.io/zone"] != "proxmox-node-01" {
		t.Errorf("Expected topology.kubernetes.io/zone label to have 'proxmox-node-01' as value, got %s", labels["topology.kubernetes.io/zone"])
	}
}
