package kubernetes

import (
	"context"
	"regexp"
	"time"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type KubernetesMock struct {
	CordonedNodes                          []string
	DeletedNodes                           []string
	AllocatedResources                     map[string]AllocatedResources
	UnschedulableResources                 UnschedulableResources
	WorkerNodesAllocatableResources        WorkerNodesAllocatableResources
	FailedSchedulingDueToControlPlaneTaint bool
	KpNodes                                []apiv1.Node
	MockClusterAllocatedResources          AllocatedResources // New field for mock

	// Enhanced scaling mock fields
	MockResourceUtilization                ResourceUtilization
	MockSchedulingErrors                   []SchedulingError
	MockDiskUtilization                    map[string]DiskUtilization
}

func (m *KubernetesMock) GetClusterAllocatedResources() (AllocatedResources, error) {
	return m.MockClusterAllocatedResources, nil
}

func (m *KubernetesMock) GetUnschedulableResources(kpNodeCores int64, kpNodeNameRegex regexp.Regexp) (UnschedulableResources, error) {
	return m.UnschedulableResources, nil
}

func (m *KubernetesMock) IsUnschedulableDueToControlPlaneTaint() (bool, error) {
	return m.FailedSchedulingDueToControlPlaneTaint, nil
}

func (m *KubernetesMock) GetWorkerNodes() ([]apiv1.Node, error) {
	return nil, nil
}

func (m *KubernetesMock) GetWorkerNodesAllocatableResources() (WorkerNodesAllocatableResources, error) {
	return m.WorkerNodesAllocatableResources, nil
}

func (m *KubernetesMock) GetKpNodes(kpNodeNameRegex regexp.Regexp) ([]apiv1.Node, error) {
	if m.KpNodes != nil {
		return m.KpNodes, nil
	}

	nodes := make([]apiv1.Node, len(m.AllocatedResources))

	idx := 0
	for k := range m.AllocatedResources {
		nodes[idx] = apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: k,
			},
		}

		idx++
	}

	return nodes, nil
}

func (m *KubernetesMock) GetKpNodesAllocatedResources(kpNodeNameRegex regexp.Regexp) (map[string]AllocatedResources, error) {
	return m.AllocatedResources, nil
}

func (m *KubernetesMock) CheckForNodeJoin(ctx context.Context, newKpNodeName string) {
}

func (m *KubernetesMock) DeleteKpNode(ctx context.Context, kpNodeName string) error {
	m.DeletedNodes = append(m.DeletedNodes, kpNodeName)
	return nil
}

func (k *KubernetesMock) LabelKpNode(kpNodeName string, newKpNodeLabels map[string]string) error {
	return nil
}

// Enhanced scaling mock methods
func (m *KubernetesMock) GetClusterResourceUtilization() (ResourceUtilization, error) {
	return m.MockResourceUtilization, nil
}

func (m *KubernetesMock) GetRecentSchedulingErrors(timeWindow time.Duration) ([]SchedulingError, error) {
	return m.MockSchedulingErrors, nil
}

func (m *KubernetesMock) GetNodeDiskUtilization() (map[string]DiskUtilization, error) {
	if m.MockDiskUtilization == nil {
		return make(map[string]DiskUtilization), nil
	}
	return m.MockDiskUtilization, nil
}
