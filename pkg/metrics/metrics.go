package metrics

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"

	"metrics-sidecar/pkg/config"
)

// ContainerMetrics 包含容器的度量指标
type ContainerMetrics struct {
	Name     string
	CPUUsage int64 // 毫核
	MemUsage int64 // MB
	Ready    bool
}

// ContainerLimits 仅包含容器的资源限制
type ContainerLimits struct {
	CPULimit int64 // 毫核
	MemLimit int64 // MB
}

// PodMetrics 包含Pod的度量指标
type PodMetrics struct {
	Name       string
	Namespace  string
	Containers map[string]*ContainerMetrics
}

// DeploymentMetrics 包含Deployment的度量指标
type DeploymentMetrics struct {
	Name              string
	Namespace         string
	Replicas          int32
	AvailableReplicas int32
}

// ResourceMetrics 包含所有资源指标的汇总
type ResourceMetrics struct {
	DeploymentReplicas          int32  `json:"deployment_replicas"`
	DeploymentAvailableReplicas int32  `json:"deployment_available_replicas"`
	ContainerName               string `json:"container_name"`
	ContainerCPULimit           int64  `json:"container_cpu_limit"` // 毫核
	ContainerMemLimit           int64  `json:"container_mem_limit"` // MB
	ContainerReady              bool   `json:"container_ready"`
	ContainerCPUUsage           int64  `json:"container_cpu_usage"` // 毫核
	ContainerMemUsage           int64  `json:"container_mem_usage"` // MB
}

// MetricsCollector 用于收集容器的度量指标
type MetricsCollector struct {
	MetricsClient *metricsclient.Clientset
	Config        *config.Config
}

// NewMetricsCollector 创建并返回一个新的MetricsCollector
func NewMetricsCollector(metricsClient *metricsclient.Clientset, cfg *config.Config) *MetricsCollector {
	return &MetricsCollector{
		MetricsClient: metricsClient,
		Config:        cfg,
	}
}

// GetPodMetrics 获取Pod的度量指标
func (m *MetricsCollector) GetPodMetrics(ctx context.Context) (*PodMetrics, error) {
	podMetrics, err := m.MetricsClient.MetricsV1beta1().PodMetricses(m.Config.Namespace).Get(ctx, m.Config.PodName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("获取Pod度量指标失败: %v", err)
	}

	result := &PodMetrics{
		Name:       m.Config.PodName,
		Namespace:  m.Config.Namespace,
		Containers: make(map[string]*ContainerMetrics),
	}

	for _, container := range podMetrics.Containers {
		if container.Name == m.Config.ContainerName {
			result.Containers[container.Name] = &ContainerMetrics{
				Name:     container.Name,
				CPUUsage: container.Usage.Cpu().MilliValue(),
				MemUsage: container.Usage.Memory().Value() / (1024 * 1024),
			}
		}
	}

	return result, nil
}
