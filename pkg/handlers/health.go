package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"metrics-sidecar/pkg/config"
	"metrics-sidecar/pkg/k8s"
	"metrics-sidecar/pkg/metrics"
)

// HealthHandler 健康检查处理器
type HealthHandler struct {
	K8sClient        *k8s.Client
	MetricsCollector *metrics.MetricsCollector
	Config           *config.Config
}

// NewHealthHandler 创建新的健康检查处理器
func NewHealthHandler(k8sClient *k8s.Client, metricsCollector *metrics.MetricsCollector, cfg *config.Config) *HealthHandler {
	return &HealthHandler{
		K8sClient:        k8sClient,
		MetricsCollector: metricsCollector,
		Config:           cfg,
	}
}

// ServeHTTP 实现http.Handler接口
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	log.Printf("开始处理健康检查请求: %s %s", r.Method, r.URL.Path)

	// 按需收集指标
	metrics, err := h.collectResourceMetrics()
	if err != nil {
		log.Printf("健康检查失败: %v", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "健康检查失败: 无法收集资源指标 - %v", err)
		return
	}

	// 输出详情
	details := make(map[string]interface{})
	details["deployment"] = map[string]interface{}{
		"name":                 h.Config.DeploymentName,
		"replicas":             metrics.DeploymentReplicas,
		"available_replicas":   metrics.DeploymentAvailableReplicas,
		"availability_percent": h.calcPodsRatio(metrics),
	}
	details["container"] = map[string]interface{}{
		"name":                 metrics.ContainerName,
		"ready":                metrics.ContainerReady,
		"memory_usage_mb":      metrics.ContainerMemUsage,
		"memory_limit_mb":      metrics.ContainerMemLimit,
		"memory_percent":       h.calcMemoryPercent(metrics),
		"cpu_usage_millicores": metrics.ContainerCPUUsage,
		"cpu_limit_millicores": metrics.ContainerCPULimit,
		"cpu_percent":          h.calcCPUPercent(metrics),
	}

	// 检查容器状态并记录结果
	status := "HEALTHY"
	statusCode := http.StatusOK
	var message string

	// 1. 检查容器是否就绪
	if !metrics.ContainerReady {
		status = "NOT_READY"
		message = fmt.Sprintf("容器 %s 尚未就绪", metrics.ContainerName)
		log.Printf("健康检查结果: %s - %s", status, message)

		details["status"] = status
		details["message"] = message
		w.WriteHeader(statusCode)
		h.writeJSONResponse(w, details)
		log.Printf("健康检查请求处理完成，耗时: %v", time.Since(startTime))
		return
	}

	// 2. 检查Pod最小可用比例
	podsRatio := h.calcPodsRatio(metrics)
	if podsRatio < h.Config.MinimumPodsToKeepPercent {
		status = "POD_SHORTAGE"
		message = fmt.Sprintf("可用Pod数量(%d/%d = %.2f%%)低于最小阈值(%.2f%%)",
			metrics.DeploymentAvailableReplicas, metrics.DeploymentReplicas,
			podsRatio, h.Config.MinimumPodsToKeepPercent)
		log.Printf("健康检查结果: %s - %s", status, message)

		details["status"] = status
		details["message"] = message
		w.WriteHeader(statusCode)
		h.writeJSONResponse(w, details)
		log.Printf("健康检查请求处理完成，耗时: %v", time.Since(startTime))
		return
	}

	// 3. 检查资源使用率
	memUsagePercent := h.calcMemoryPercent(metrics)
	cpuUsagePercent := h.calcCPUPercent(metrics)

	// 如果CPU和内存同时超过阈值，返回400
	if memUsagePercent > h.Config.ResourceThresholdMemoryPercent &&
		cpuUsagePercent > h.Config.ResourceThresholdCPUPercent {
		status = "RESOURCE_EXHAUSTED"
		statusCode = http.StatusBadRequest
		message = fmt.Sprintf("资源使用率过高: 内存使用率 %.2f%% (阈值: %.2f%%), CPU使用率 %.2f%% (阈值: %.2f%%)",
			memUsagePercent, h.Config.ResourceThresholdMemoryPercent,
			cpuUsagePercent, h.Config.ResourceThresholdCPUPercent)
		log.Printf("健康检查结果: %s - %s", status, message)

		details["status"] = status
		details["message"] = message
		w.WriteHeader(statusCode)
		h.writeJSONResponse(w, details)
		log.Printf("健康检查请求处理完成，耗时: %v", time.Since(startTime))
		return
	}

	// 一切正常
	message = fmt.Sprintf("健康检查通过: 内存使用率 %.2f%%, CPU使用率 %.2f%%, Pod可用率 %.2f%%",
		memUsagePercent, cpuUsagePercent, podsRatio)
	log.Printf("健康检查结果: %s - %s", status, message)

	details["status"] = status
	details["message"] = message
	w.WriteHeader(statusCode)
	h.writeJSONResponse(w, details)
	log.Printf("健康检查请求处理完成，耗时: %v", time.Since(startTime))
}

// 收集资源指标
func (h *HealthHandler) collectResourceMetrics() (*metrics.ResourceMetrics, error) {
	log.Printf("开始收集资源指标")
	startTime := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	metrics := &metrics.ResourceMetrics{
		ContainerName: h.Config.ContainerName,
	}

	// 获取Deployment信息
	log.Printf("获取Deployment[%s]信息", h.Config.DeploymentName)
	deploymentInfo, err := h.K8sClient.GetDeploymentInfo(ctx)
	if err != nil {
		log.Printf("获取Deployment信息失败: %v", err)
	} else if deploymentInfo != nil {
		metrics.DeploymentReplicas = deploymentInfo.Replicas
		metrics.DeploymentAvailableReplicas = deploymentInfo.AvailableReplicas
		log.Printf("Deployment信息: 副本数=%d, 可用副本数=%d",
			deploymentInfo.Replicas, deploymentInfo.AvailableReplicas)
	}

	// 获取容器资源限制 (直接使用已缓存的值)
	log.Printf("获取容器[%s]资源限制", h.Config.ContainerName)
	containerLimits, err := h.K8sClient.GetContainerLimits(ctx)
	if err != nil {
		log.Printf("获取容器资源限制失败: %v", err)
		return nil, fmt.Errorf("无法获取容器资源限制: %v", err)
	}
	metrics.ContainerCPULimit = containerLimits.CPULimit
	metrics.ContainerMemLimit = containerLimits.MemLimit
	log.Printf("容器资源限制: CPU=%dm, 内存=%dMi",
		containerLimits.CPULimit, containerLimits.MemLimit)

	// 获取Pod信息
	log.Printf("获取Pod[%s]信息", h.Config.PodName)
	podInfo, err := h.K8sClient.GetPodInfo(ctx)
	if err != nil {
		log.Printf("获取Pod信息失败: %v", err)
	} else if podInfo != nil && podInfo.Containers != nil {
		container := podInfo.Containers[h.Config.ContainerName]
		if container != nil {
			metrics.ContainerReady = container.Ready
			log.Printf("容器就绪状态: %v", container.Ready)
		}
	}

	// 获取Pod度量指标
	log.Printf("获取Pod度量指标")
	podMetrics, err := h.MetricsCollector.GetPodMetrics(ctx)
	if err != nil {
		log.Printf("获取Pod度量指标失败: %v", err)
	} else if podMetrics != nil && podMetrics.Containers != nil {
		container := podMetrics.Containers[h.Config.ContainerName]
		if container != nil {
			metrics.ContainerCPUUsage = container.CPUUsage
			metrics.ContainerMemUsage = container.MemUsage
			log.Printf("容器资源使用: CPU=%dm, 内存=%dMi",
				container.CPUUsage, container.MemUsage)
		}
	}

	log.Printf("资源指标收集完成，耗时: %v", time.Since(startTime))
	return metrics, nil
}

// 计算Pod可用率
func (h *HealthHandler) calcPodsRatio(metrics *metrics.ResourceMetrics) float64 {
	if metrics.DeploymentReplicas <= 0 {
		return 0.0
	}
	return float64(metrics.DeploymentAvailableReplicas) / float64(metrics.DeploymentReplicas) * 100
}

// 计算内存使用率
func (h *HealthHandler) calcMemoryPercent(metrics *metrics.ResourceMetrics) float64 {
	if metrics.ContainerMemLimit <= 0 {
		return 0.0
	}
	return float64(metrics.ContainerMemUsage) / float64(metrics.ContainerMemLimit) * 100
}

// 计算CPU使用率
func (h *HealthHandler) calcCPUPercent(metrics *metrics.ResourceMetrics) float64 {
	if metrics.ContainerCPULimit <= 0 {
		return 0.0
	}
	return float64(metrics.ContainerCPUUsage) / float64(metrics.ContainerCPULimit) * 100
}

// 输出JSON响应
func (h *HealthHandler) writeJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Printf("序列化JSON失败: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(jsonData)
}
