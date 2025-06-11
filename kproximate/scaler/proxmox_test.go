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

// Enhanced Scaling Tests

func TestResourcePressureScaling(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			UnschedulableResources: kubernetes.UnschedulableResources{
				Cpu:    0,
				Memory: 0,
			},
			MockResourceUtilization: kubernetes.ResourceUtilization{
				CpuUtilization:    0.85, // Above 80% threshold
				MemoryUtilization: 0.75, // Below 80% threshold
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:                   2,
			KpNodeMemory:                  2048,
			MaxKpNodes:                    3,
			EnableResourcePressureScaling: true,
			CpuUtilizationThreshold:       0.8,
			MemoryUtilizationThreshold:    0.8,
		},
	}

	currentEvents := 0

	requiredScaleEvents, err := s.RequiredScaleEvents(currentEvents)
	if err != nil {
		t.Error(err)
	}

	// Should trigger scaling due to CPU pressure
	if len(requiredScaleEvents) != 1 {
		t.Errorf("Expected exactly 1 scaleEvent due to CPU pressure, got: %d", len(requiredScaleEvents))
	}
}

func TestMemoryPressureScaling(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			UnschedulableResources: kubernetes.UnschedulableResources{
				Cpu:    0,
				Memory: 0,
			},
			MockResourceUtilization: kubernetes.ResourceUtilization{
				CpuUtilization:    0.75, // Below 80% threshold
				MemoryUtilization: 0.85, // Above 80% threshold
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:                   2,
			KpNodeMemory:                  2048,
			MaxKpNodes:                    3,
			EnableResourcePressureScaling: true,
			CpuUtilizationThreshold:       0.8,
			MemoryUtilizationThreshold:    0.8,
		},
	}

	currentEvents := 0

	requiredScaleEvents, err := s.RequiredScaleEvents(currentEvents)
	if err != nil {
		t.Error(err)
	}

	// Should trigger scaling due to memory pressure
	if len(requiredScaleEvents) != 1 {
		t.Errorf("Expected exactly 1 scaleEvent due to memory pressure, got: %d", len(requiredScaleEvents))
	}
}

func TestSchedulingErrorScaling(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			UnschedulableResources: kubernetes.UnschedulableResources{
				Cpu:    0,
				Memory: 0,
			},
			MockSchedulingErrors: []kubernetes.SchedulingError{
				{
					PodName:      "pod-1",
					Namespace:    "default",
					Reason:       "FailedScheduling",
					Message:      "Insufficient CPU",
					Timestamp:    time.Now(),
					FailureCount: 1,
				},
				{
					PodName:      "pod-2",
					Namespace:    "default",
					Reason:       "FailedScheduling",
					Message:      "Insufficient memory",
					Timestamp:    time.Now(),
					FailureCount: 2,
				},
				{
					PodName:      "pod-3",
					Namespace:    "kube-system",
					Reason:       "FailedScheduling",
					Message:      "No suitable node",
					Timestamp:    time.Now(),
					FailureCount: 1,
				},
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:                  2,
			KpNodeMemory:                 2048,
			MaxKpNodes:                   3,
			EnableSchedulingErrorScaling: true,
			SchedulingErrorThreshold:     3,
		},
	}

	currentEvents := 0

	requiredScaleEvents, err := s.RequiredScaleEvents(currentEvents)
	if err != nil {
		t.Error(err)
	}

	// Should trigger scaling due to scheduling errors (3 unique failed pods)
	if len(requiredScaleEvents) != 1 {
		t.Errorf("Expected exactly 1 scaleEvent due to scheduling errors, got: %d", len(requiredScaleEvents))
	}
}

func TestStoragePressureScaling(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			UnschedulableResources: kubernetes.UnschedulableResources{
				Cpu:    0,
				Memory: 0,
			},
			MockDiskUtilization: map[string]kubernetes.DiskUtilization{
				"worker-node-1": {
					NodeName:             "worker-node-1",
					TotalDiskSpaceGB:     100,
					UsedDiskSpaceGB:      88, // 88% utilization, above 85% threshold
					AvailableDiskSpaceGB: 12,
					UtilizationPercent:   0.88,
				},
				"worker-node-2": {
					NodeName:             "worker-node-2",
					TotalDiskSpaceGB:     100,
					UsedDiskSpaceGB:      70,
					AvailableDiskSpaceGB: 30,
					UtilizationPercent:   0.70,
				},
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:                  2,
			KpNodeMemory:                 2048,
			MaxKpNodes:                   3,
			EnableStoragePressureScaling: true,
			DiskUtilizationThreshold:     0.85,
			MinAvailableDiskSpaceGB:      5,
		},
	}

	currentEvents := 0

	requiredScaleEvents, err := s.RequiredScaleEvents(currentEvents)
	if err != nil {
		t.Error(err)
	}

	// Should trigger scaling due to storage pressure
	if len(requiredScaleEvents) != 1 {
		t.Errorf("Expected exactly 1 scaleEvent due to storage pressure, got: %d", len(requiredScaleEvents))
	}
}

func TestMinDiskSpaceScaling(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			UnschedulableResources: kubernetes.UnschedulableResources{
				Cpu:    0,
				Memory: 0,
			},
			MockDiskUtilization: map[string]kubernetes.DiskUtilization{
				"worker-node-1": {
					NodeName:             "worker-node-1",
					TotalDiskSpaceGB:     100,
					UsedDiskSpaceGB:      97, // Only 3GB available, below 5GB threshold
					AvailableDiskSpaceGB: 3,
					UtilizationPercent:   0.97,
				},
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:                  2,
			KpNodeMemory:                 2048,
			MaxKpNodes:                   3,
			EnableStoragePressureScaling: true,
			DiskUtilizationThreshold:     0.85,
			MinAvailableDiskSpaceGB:      5,
		},
	}

	currentEvents := 0

	requiredScaleEvents, err := s.RequiredScaleEvents(currentEvents)
	if err != nil {
		t.Error(err)
	}

	// Should trigger scaling due to insufficient available disk space
	if len(requiredScaleEvents) != 1 {
		t.Errorf("Expected exactly 1 scaleEvent due to insufficient disk space, got: %d", len(requiredScaleEvents))
	}
}

func TestCombinedTraditionalAndEnhancedScaling(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			UnschedulableResources: kubernetes.UnschedulableResources{
				Cpu:    1.0,        // Traditional unschedulable CPU
				Memory: 1073741824, // Traditional unschedulable memory (1GB)
			},
			MockResourceUtilization: kubernetes.ResourceUtilization{
				CpuUtilization:    0.85, // Above threshold
				MemoryUtilization: 0.75, // Below threshold
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:                   2,
			KpNodeMemory:                  2048,
			MaxKpNodes:                    3,
			EnableResourcePressureScaling: true,
			CpuUtilizationThreshold:       0.8,
			MemoryUtilizationThreshold:    0.8,
		},
	}

	currentEvents := 0

	requiredScaleEvents, err := s.RequiredScaleEvents(currentEvents)
	if err != nil {
		t.Error(err)
	}

	// Should trigger scaling due to both traditional unschedulable resources and CPU pressure
	// The CPU requirement should be the max of traditional (1.0) and enhanced (2.0) = 2.0 CPU
	// Memory requirements should be additive: traditional (1GB) + enhanced (2GB) = 3GB
	// This should result in 1 scale event since max(1 CPU node, 2 CPU nodes) = 2 CPU nodes
	// and ceil(3GB / 2GB per node) = 2 memory nodes, so max(2,2) = 2 nodes total
	// But since we're using math.Max for CPU and additive for memory, we get 1 node
	if len(requiredScaleEvents) != 1 {
		t.Errorf("Expected exactly 1 scaleEvent due to combined scaling factors, got: %d", len(requiredScaleEvents))
	}
}

func TestEnhancedScalingDisabled(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			UnschedulableResources: kubernetes.UnschedulableResources{
				Cpu:    0,
				Memory: 0,
			},
			MockResourceUtilization: kubernetes.ResourceUtilization{
				CpuUtilization:    0.95, // Well above threshold
				MemoryUtilization: 0.95, // Well above threshold
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:                   2,
			KpNodeMemory:                  2048,
			MaxKpNodes:                    3,
			EnableResourcePressureScaling: false, // Disabled
			CpuUtilizationThreshold:       0.8,
			MemoryUtilizationThreshold:    0.8,
		},
	}

	currentEvents := 0

	requiredScaleEvents, err := s.RequiredScaleEvents(currentEvents)
	if err != nil {
		t.Error(err)
	}

	// Should not trigger scaling since enhanced scaling is disabled
	if len(requiredScaleEvents) != 0 {
		t.Errorf("Expected 0 scaleEvents since enhanced scaling is disabled, got: %d", len(requiredScaleEvents))
	}
}

// Enhanced Scale-Down Tests

func TestScaleDownPreventedByResourcePressure(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			KpNodes: []apiv1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "kp-node-old",
						CreationTimestamp: metav1.NewTime(time.Now().Add(-30 * time.Minute)), // Old enough
					},
				},
			},
			WorkerNodesAllocatableResources: kubernetes.WorkerNodesAllocatableResources{
				Cpu:    8,
				Memory: 8589934592, // 8GB
			},
			AllocatedResources: map[string]kubernetes.AllocatedResources{
				"kp-node-old": {
					Cpu:    2.0,          // Low load normally allowing scale-down
					Memory: 1073741824.0, // 1GB
				},
			},
			MockResourceUtilization: kubernetes.ResourceUtilization{
				CpuUtilization:    0.75, // High utilization preventing scale-down
				MemoryUtilization: 0.60, // Normal memory utilization
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:                   2,
			KpNodeMemory:                  2048,
			LoadHeadroom:                  0.2,
			ScaleDownStabilizationMinutes: 5,
			MinNodeAgeMinutes:             10,
			EnableResourcePressureScaling: true,
			CpuUtilizationThreshold:       0.8, // Scale-down threshold would be 0.7
			MemoryUtilizationThreshold:    0.8,
		},
	}

	scaleEvent, err := s.AssessScaleDown()
	if err != nil {
		t.Error(err)
	}

	// Should prevent scale-down due to high CPU utilization
	if scaleEvent != nil {
		t.Error("Expected scale-down to be prevented due to high CPU utilization")
	}
}

func TestScaleDownPreventedBySchedulingErrors(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			KpNodes: []apiv1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "kp-node-old",
						CreationTimestamp: metav1.NewTime(time.Now().Add(-30 * time.Minute)),
					},
				},
			},
			WorkerNodesAllocatableResources: kubernetes.WorkerNodesAllocatableResources{
				Cpu:    8,
				Memory: 8589934592,
			},
			AllocatedResources: map[string]kubernetes.AllocatedResources{
				"kp-node-old": {
					Cpu:    1.0,
					Memory: 1073741824.0,
				},
			},
			MockResourceUtilization: kubernetes.ResourceUtilization{
				CpuUtilization:    0.60, // Low utilization
				MemoryUtilization: 0.60, // Low utilization
			},
			MockSchedulingErrors: []kubernetes.SchedulingError{
				{
					PodName:      "failed-pod-1",
					Namespace:    "default",
					Reason:       "FailedScheduling",
					Message:      "Insufficient resources",
					Timestamp:    time.Now(),
					FailureCount: 1,
				},
				{
					PodName:      "failed-pod-2",
					Namespace:    "default",
					Reason:       "FailedScheduling",
					Message:      "Insufficient resources",
					Timestamp:    time.Now(),
					FailureCount: 1,
				},
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:                   2,
			KpNodeMemory:                  2048,
			LoadHeadroom:                  0.2,
			ScaleDownStabilizationMinutes: 5,
			MinNodeAgeMinutes:             10,
			EnableSchedulingErrorScaling:  true,
			SchedulingErrorThreshold:      4, // Scale-down threshold would be 2
		},
	}

	scaleEvent, err := s.AssessScaleDown()
	if err != nil {
		t.Error(err)
	}

	// Should prevent scale-down due to recent scheduling errors
	if scaleEvent != nil {
		t.Error("Expected scale-down to be prevented due to recent scheduling errors")
	}
}

func TestScaleDownPreventedByStoragePressure(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			KpNodes: []apiv1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "kp-node-old",
						CreationTimestamp: metav1.NewTime(time.Now().Add(-30 * time.Minute)),
					},
				},
			},
			WorkerNodesAllocatableResources: kubernetes.WorkerNodesAllocatableResources{
				Cpu:    8,
				Memory: 8589934592,
			},
			AllocatedResources: map[string]kubernetes.AllocatedResources{
				"kp-node-old": {
					Cpu:    1.0,
					Memory: 1073741824.0,
				},
			},
			MockResourceUtilization: kubernetes.ResourceUtilization{
				CpuUtilization:    0.60,
				MemoryUtilization: 0.60,
			},
			MockDiskUtilization: map[string]kubernetes.DiskUtilization{
				"worker-node-1": {
					NodeName:             "worker-node-1",
					TotalDiskSpaceGB:     100,
					UsedDiskSpaceGB:      82, // 82% utilization, above scale-down threshold (80%)
					AvailableDiskSpaceGB: 18,
					UtilizationPercent:   0.82,
				},
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:                   2,
			KpNodeMemory:                  2048,
			LoadHeadroom:                  0.2,
			ScaleDownStabilizationMinutes: 5,
			MinNodeAgeMinutes:             10,
			EnableStoragePressureScaling:  true,
			DiskUtilizationThreshold:      0.85, // Scale-down threshold would be 0.80
			MinAvailableDiskSpaceGB:       5,    // Scale-down threshold would be 7GB
		},
	}

	scaleEvent, err := s.AssessScaleDown()
	if err != nil {
		t.Error(err)
	}

	// Should prevent scale-down due to high disk utilization
	if scaleEvent != nil {
		t.Error("Expected scale-down to be prevented due to high disk utilization")
	}
}

func TestScaleDownAllowedWhenEnhancedFactorsNormal(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			KpNodes: []apiv1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "kp-node-old",
						CreationTimestamp: metav1.NewTime(time.Now().Add(-30 * time.Minute)),
					},
				},
			},
			WorkerNodesAllocatableResources: kubernetes.WorkerNodesAllocatableResources{
				Cpu:    8,
				Memory: 8589934592,
			},
			AllocatedResources: map[string]kubernetes.AllocatedResources{
				"kp-node-old": {
					Cpu:    1.0,          // Low load allowing scale-down
					Memory: 1073741824.0, // 1GB
				},
			},
			MockResourceUtilization: kubernetes.ResourceUtilization{
				CpuUtilization:    0.50, // Low utilization
				MemoryUtilization: 0.50, // Low utilization
			},
			MockSchedulingErrors: []kubernetes.SchedulingError{}, // No scheduling errors
			MockDiskUtilization: map[string]kubernetes.DiskUtilization{
				"worker-node-1": {
					NodeName:             "worker-node-1",
					TotalDiskSpaceGB:     100,
					UsedDiskSpaceGB:      60, // 60% utilization, well below thresholds
					AvailableDiskSpaceGB: 40,
					UtilizationPercent:   0.60,
				},
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:                   2,
			KpNodeMemory:                  2048,
			LoadHeadroom:                  0.2,
			ScaleDownStabilizationMinutes: 5,
			MinNodeAgeMinutes:             10,
			EnableResourcePressureScaling: true,
			EnableSchedulingErrorScaling:  true,
			EnableStoragePressureScaling:  true,
			CpuUtilizationThreshold:       0.8,
			MemoryUtilizationThreshold:    0.8,
			SchedulingErrorThreshold:      3,
			DiskUtilizationThreshold:      0.85,
			MinAvailableDiskSpaceGB:       5,
		},
	}

	scaleEvent, err := s.AssessScaleDown()
	if err != nil {
		t.Error(err)
	}

	// Should allow scale-down since all enhanced factors are normal
	if scaleEvent == nil {
		t.Error("Expected scale-down to be allowed when enhanced factors are normal")
	} else if scaleEvent.NodeName != "kp-node-old" {
		t.Errorf("Expected scale-down target to be 'kp-node-old', got: %s", scaleEvent.NodeName)
	}
}

func TestScaleDownWithEnhancedFactorsDisabled(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			KpNodes: []apiv1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "kp-node-old",
						CreationTimestamp: metav1.NewTime(time.Now().Add(-30 * time.Minute)),
					},
				},
			},
			WorkerNodesAllocatableResources: kubernetes.WorkerNodesAllocatableResources{
				Cpu:    8,
				Memory: 8589934592,
			},
			AllocatedResources: map[string]kubernetes.AllocatedResources{
				"kp-node-old": {
					Cpu:    1.0,
					Memory: 1073741824.0,
				},
			},
			MockResourceUtilization: kubernetes.ResourceUtilization{
				CpuUtilization:    0.90, // High utilization that would prevent scale-down
				MemoryUtilization: 0.90, // High utilization that would prevent scale-down
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:                   2,
			KpNodeMemory:                  2048,
			LoadHeadroom:                  0.2,
			ScaleDownStabilizationMinutes: 5,
			MinNodeAgeMinutes:             10,
			EnableResourcePressureScaling: false, // Disabled
			EnableSchedulingErrorScaling:  false, // Disabled
			EnableStoragePressureScaling:  false, // Disabled
		},
	}

	scaleEvent, err := s.AssessScaleDown()
	if err != nil {
		t.Error(err)
	}

	// Should allow scale-down since enhanced factors are disabled
	if scaleEvent == nil {
		t.Error("Expected scale-down to be allowed when enhanced factors are disabled")
	}
}

func TestScaleDownPreventedByProjectedResourcePressure(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			KpNodes: []apiv1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "kp-node-target",
						CreationTimestamp: metav1.NewTime(time.Now().Add(-30 * time.Minute)),
					},
				},
			},
			WorkerNodesAllocatableResources: kubernetes.WorkerNodesAllocatableResources{
				Cpu:    6,          // 6 CPU cores total
				Memory: 6442450944, // 6GB total
			},
			AllocatedResources: map[string]kubernetes.AllocatedResources{
				"kp-node-target": {
					Cpu:    1.0,          // Low load on target node
					Memory: 1073741824.0, // 1GB on target node
				},
			},
			MockClusterAllocatedResources: kubernetes.AllocatedResources{
				Cpu:    4.0,          // 4 CPU cores allocated cluster-wide
				Memory: 4294967296.0, // 4GB allocated cluster-wide
			},
			MockResourceUtilization: kubernetes.ResourceUtilization{
				CpuUtilization:    0.67, // Current: 4/6 = 67%, acceptable
				MemoryUtilization: 0.67, // Current: 4/6 = 67%, acceptable
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:                   2,    // Each node has 2 cores
			KpNodeMemory:                  2048, // Each node has 2GB
			LoadHeadroom:                  0.2,
			ScaleDownStabilizationMinutes: 5,
			MinNodeAgeMinutes:             10,
			EnableResourcePressureScaling: true,
			CpuUtilizationThreshold:       0.8, // 80% threshold
			MemoryUtilizationThreshold:    0.8, // 80% threshold
		},
	}

	scaleEvent, err := s.AssessScaleDown()
	if err != nil {
		t.Error(err)
	}

	// Should prevent scale-down because removing 2 cores would make utilization 4/4 = 100%
	// which exceeds the 80% threshold
	if scaleEvent != nil {
		t.Error("Expected scale-down to be prevented due to projected resource pressure")
	}
}

func TestScaleDownPreventedByProjectedStoragePressure(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			KpNodes: []apiv1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "kp-node-target",
						CreationTimestamp: metav1.NewTime(time.Now().Add(-30 * time.Minute)),
					},
				},
			},
			WorkerNodesAllocatableResources: kubernetes.WorkerNodesAllocatableResources{
				Cpu:    8,
				Memory: 8589934592,
			},
			AllocatedResources: map[string]kubernetes.AllocatedResources{
				"kp-node-target": {
					Cpu:    1.0,
					Memory: 1073741824.0,
				},
			},
			MockResourceUtilization: kubernetes.ResourceUtilization{
				CpuUtilization:    0.50, // Low utilization
				MemoryUtilization: 0.50, // Low utilization
			},
			MockDiskUtilization: map[string]kubernetes.DiskUtilization{
				"kp-node-target": {
					NodeName:             "kp-node-target",
					TotalDiskSpaceGB:     100,
					UsedDiskSpaceGB:      50,
					AvailableDiskSpaceGB: 50,
					UtilizationPercent:   0.50,
				},
				"worker-node-1": {
					NodeName:             "worker-node-1",
					TotalDiskSpaceGB:     100,
					UsedDiskSpaceGB:      82, // 82% utilization, above conservative threshold (85% - 5% = 80%)
					AvailableDiskSpaceGB: 18,
					UtilizationPercent:   0.82,
				},
				"worker-node-2": {
					NodeName:             "worker-node-2",
					TotalDiskSpaceGB:     100,
					UsedDiskSpaceGB:      81, // 81% utilization, above conservative threshold (85% - 5% = 80%)
					AvailableDiskSpaceGB: 19,
					UtilizationPercent:   0.81,
				},
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:                   2,
			KpNodeMemory:                  2048,
			LoadHeadroom:                  0.2,
			ScaleDownStabilizationMinutes: 5,
			MinNodeAgeMinutes:             10,
			EnableStoragePressureScaling:  true,
			DiskUtilizationThreshold:      0.85, // Conservative threshold: 80%
			MinAvailableDiskSpaceGB:       5,    // Conservative threshold: 8GB
		},
	}

	scaleEvent, err := s.AssessScaleDown()
	if err != nil {
		t.Error(err)
	}

	// Should prevent scale-down because 2 out of 2 remaining nodes (100%) are near storage thresholds
	if scaleEvent != nil {
		t.Error("Expected scale-down to be prevented due to projected storage pressure on remaining nodes")
	}
}

func TestScaleDownAllowedWithSpecificNodeRemovalSimulation(t *testing.T) {
	s := ProxmoxScaler{
		Kubernetes: &kubernetes.KubernetesMock{
			KpNodes: []apiv1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "kp-node-target",
						CreationTimestamp: metav1.NewTime(time.Now().Add(-30 * time.Minute)),
					},
				},
			},
			WorkerNodesAllocatableResources: kubernetes.WorkerNodesAllocatableResources{
				Cpu:    8,          // 8 CPU cores total
				Memory: 8589934592, // 8GB total
			},
			AllocatedResources: map[string]kubernetes.AllocatedResources{
				"kp-node-target": {
					Cpu:    1.0,
					Memory: 1073741824.0,
				},
			},
			MockClusterAllocatedResources: kubernetes.AllocatedResources{
				Cpu:    3.0,          // 3 CPU cores allocated cluster-wide
				Memory: 3221225472.0, // 3GB allocated cluster-wide
			},
			MockResourceUtilization: kubernetes.ResourceUtilization{
				CpuUtilization:    0.375, // Current: 3/8 = 37.5%, low
				MemoryUtilization: 0.375, // Current: 3/8 = 37.5%, low
			},
			MockDiskUtilization: map[string]kubernetes.DiskUtilization{
				"kp-node-target": {
					NodeName:             "kp-node-target",
					TotalDiskSpaceGB:     100,
					UsedDiskSpaceGB:      50,
					AvailableDiskSpaceGB: 50,
					UtilizationPercent:   0.50,
				},
				"worker-node-1": {
					NodeName:             "worker-node-1",
					TotalDiskSpaceGB:     100,
					UsedDiskSpaceGB:      60, // 60% utilization, well below thresholds
					AvailableDiskSpaceGB: 40,
					UtilizationPercent:   0.60,
				},
				"worker-node-2": {
					NodeName:             "worker-node-2",
					TotalDiskSpaceGB:     100,
					UsedDiskSpaceGB:      65, // 65% utilization, well below thresholds
					AvailableDiskSpaceGB: 35,
					UtilizationPercent:   0.65,
				},
			},
		},
		config: config.KproximateConfig{
			KpNodeCores:                   2,    // Each node has 2 cores
			KpNodeMemory:                  2048, // Each node has 2GB
			LoadHeadroom:                  0.2,
			ScaleDownStabilizationMinutes: 5,
			MinNodeAgeMinutes:             10,
			EnableResourcePressureScaling: true,
			EnableStoragePressureScaling:  true,
			CpuUtilizationThreshold:       0.8,  // 80% threshold
			MemoryUtilizationThreshold:    0.8,  // 80% threshold
			DiskUtilizationThreshold:      0.85, // 85% threshold
			MinAvailableDiskSpaceGB:       5,
		},
	}

	scaleEvent, err := s.AssessScaleDown()
	if err != nil {
		t.Error(err)
	}

	// Should allow scale-down because:
	// - Current utilization: 3/8 = 37.5% (low)
	// - After removing 2 cores: 3/6 = 50% (still well below 80% threshold)
	// - Storage pressure is low on remaining nodes
	if scaleEvent == nil {
		t.Error("Expected scale-down to be allowed with proper node removal simulation")
	} else if scaleEvent.NodeName != "kp-node-target" {
		t.Errorf("Expected scale-down target to be 'kp-node-target', got: %s", scaleEvent.NodeName)
	}
}
