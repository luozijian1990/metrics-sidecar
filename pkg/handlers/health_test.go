package handlers

import (
	"testing"

	"metrics-sidecar/pkg/config"
	"metrics-sidecar/pkg/metrics"
)

// 测试健康状态计算函数
func TestCalcFunctions(t *testing.T) {
	// 创建HealthHandler
	cfg := &config.Config{
		ResourceThresholdMemoryPercent: 80.0,
		ResourceThresholdCPUPercent:    80.0,
		MinimumPodsToKeepPercent:       50.0,
	}

	handler := &HealthHandler{
		Config: cfg,
	}

	// 测试Pod比例计算
	t.Run("calcPodsRatio", func(t *testing.T) {
		metrics := &metrics.ResourceMetrics{
			DeploymentReplicas:          10,
			DeploymentAvailableReplicas: 5,
		}

		ratio := handler.calcPodsRatio(metrics)
		if ratio != 50.0 {
			t.Errorf("calcPodsRatio返回 %f; 期望 50.0", ratio)
		}

		// 测试零值处理
		metrics.DeploymentReplicas = 0
		ratio = handler.calcPodsRatio(metrics)
		if ratio != 0.0 {
			t.Errorf("当副本数为0时calcPodsRatio返回 %f; 期望 0.0", ratio)
		}
	})

	// 测试内存使用率计算
	t.Run("calcMemoryPercent", func(t *testing.T) {
		metrics := &metrics.ResourceMetrics{
			ContainerMemLimit: 1000,
			ContainerMemUsage: 500,
		}

		percent := handler.calcMemoryPercent(metrics)
		if percent != 50.0 {
			t.Errorf("calcMemoryPercent返回 %f; 期望 50.0", percent)
		}

		// 测试零值处理
		metrics.ContainerMemLimit = 0
		percent = handler.calcMemoryPercent(metrics)
		if percent != 0.0 {
			t.Errorf("当内存限制为0时calcMemoryPercent返回 %f; 期望 0.0", percent)
		}
	})

	// 测试CPU使用率计算
	t.Run("calcCPUPercent", func(t *testing.T) {
		metrics := &metrics.ResourceMetrics{
			ContainerCPULimit: 1000,
			ContainerCPUUsage: 800,
		}

		percent := handler.calcCPUPercent(metrics)
		if percent != 80.0 {
			t.Errorf("calcCPUPercent返回 %f; 期望 80.0", percent)
		}

		// 测试零值处理
		metrics.ContainerCPULimit = 0
		percent = handler.calcCPUPercent(metrics)
		if percent != 0.0 {
			t.Errorf("当CPU限制为0时calcCPUPercent返回 %f; 期望 0.0", percent)
		}
	})
}
