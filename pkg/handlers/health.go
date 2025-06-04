package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"metrics-sidecar/pkg/config"
	"metrics-sidecar/pkg/k8s"
	"metrics-sidecar/pkg/logger"
	"metrics-sidecar/pkg/metrics"

	"github.com/sirupsen/logrus"
)

var (
	resourceOverLoaded bool
	rng                *rand.Rand // 用于生成随机数的随机数生成器

	// 标记是否继续进行随机决策，初始为true表示需要随机决策
	shouldRandomize = true

	// 健康检查处理器的日志器
	log = logger.GetLogger("health")
)

// 初始化随机数生成器
func init() {
	// 使用当前时间作为随机数种子，确保每次运行程序时都有不同的随机序列
	source := rand.NewSource(time.Now().UnixNano())
	rng = rand.New(source)
}

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
	log.WithFields(logrus.Fields{
		"method": r.Method,
		"path":   r.URL.Path,
	}).Info("开始处理健康检查请求")

	// 按需收集指标
	resourceMetrics, err := h.collectResourceMetrics()
	if err != nil {
		log.WithError(err).Error("健康检查失败")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "健康检查失败: 无法收集资源指标 - %v", err)
		return
	}

	// 输出详情
	details := make(map[string]interface{})
	details["deployment"] = map[string]interface{}{
		"name":                 h.Config.DeploymentName,
		"replicas":             resourceMetrics.DeploymentReplicas,
		"available_replicas":   resourceMetrics.DeploymentAvailableReplicas,
		"availability_percent": h.calcPodsRatio(resourceMetrics),
	}
	details["container"] = map[string]interface{}{
		"name":                 resourceMetrics.ContainerName,
		"ready":                resourceMetrics.ContainerReady,
		"memory_usage_mb":      resourceMetrics.ContainerMemUsage,
		"memory_limit_mb":      resourceMetrics.ContainerMemLimit,
		"memory_percent":       h.calcMemoryPercent(resourceMetrics),
		"cpu_usage_millicores": resourceMetrics.ContainerCPUUsage,
		"cpu_limit_millicores": resourceMetrics.ContainerCPULimit,
		"cpu_percent":          h.calcCPUPercent(resourceMetrics),
	}

	// 检查容器状态并记录结果
	status := "HEALTHY"
	statusCode := http.StatusOK
	var message string

	// 1. 检查容器是否就绪
	if !resourceMetrics.ContainerReady {
		status = "NOT_READY"
		message = fmt.Sprintf("容器 %s 尚未就绪", resourceMetrics.ContainerName)
		log.WithField("status", status).Info(message)

		details["status"] = status
		details["message"] = message
		w.WriteHeader(statusCode)
		h.writeJSONResponse(w, details)
		log.WithField("duration", time.Since(startTime)).Info("健康检查请求处理完成")
		return
	}

	// 2. 检查Pod最小可用比例
	podsRatio := h.calcPodsRatio(resourceMetrics)
	if podsRatio < h.Config.MinimumPodsToKeepPercent {
		status = "POD_SHORTAGE"
		message = fmt.Sprintf("可用Pod数量(%d/%d = %.2f%%)低于最小阈值(%.2f%%)",
			resourceMetrics.DeploymentAvailableReplicas, resourceMetrics.DeploymentReplicas,
			podsRatio, h.Config.MinimumPodsToKeepPercent)
		log.WithField("status", status).Info(message)

		details["status"] = status
		details["message"] = message
		w.WriteHeader(statusCode)
		h.writeJSONResponse(w, details)
		log.WithField("duration", time.Since(startTime)).Info("健康检查请求处理完成")
		return
	}

	// 3. 检查资源使用率
	memUsagePercent := h.calcMemoryPercent(resourceMetrics)
	cpuUsagePercent := h.calcCPUPercent(resourceMetrics)

	// 判断资源是否过载
	resourceOverLoaded = memUsagePercent > h.Config.ResourceThresholdMemoryPercent &&
		cpuUsagePercent > h.Config.ResourceThresholdCPUPercent

	// 如果资源过载，进行随机退避决策
	if resourceOverLoaded {
		// 如果允许随机决策，执行随机逻辑
		if shouldRandomize {
			// 生成0-100的随机数
			randomValue := rng.Float64() * 100

			// 如果随机值大于阈值，设置为不再随机并返回不健康状态
			if randomValue > h.Config.MinimumPodsToKeepPercent {
				shouldRandomize = false // 第一次随机大于阈值，后续都不再随机
				log.WithFields(logrus.Fields{
					"random_value": randomValue,
					"threshold":    h.Config.MinimumPodsToKeepPercent,
				}).Info("首次随机决策: 固定拒绝流量")

				status = "RESOURCE_EXHAUSTED"
				statusCode = http.StatusBadRequest
				message = fmt.Sprintf("资源使用率过高: 内存使用率 %.2f%% (阈值: %.2f%%), CPU使用率 %.2f%% (阈值: %.2f%%)",
					memUsagePercent, h.Config.ResourceThresholdMemoryPercent,
					cpuUsagePercent, h.Config.ResourceThresholdCPUPercent)
				log.WithFields(logrus.Fields{
					"status":  status,
					"message": message,
				}).Info("健康检查结果: 固定拒绝流量")
			} else {
				// 随机值小于等于阈值，保持允许随机状态，本次返回健康
				log.WithFields(logrus.Fields{
					"random_value": randomValue,
					"threshold":    h.Config.MinimumPodsToKeepPercent,
				}).Info("首次随机决策: 继续随机决策")
				status = "RESOURCE_OVERLOADED_BUT_KEEPING"
				message = fmt.Sprintf("资源使用率过高但随机退避生效: 内存使用率 %.2f%%, CPU使用率 %.2f%%, 随机值 %.2f",
					memUsagePercent, cpuUsagePercent, randomValue)
			}
		} else {
			// 之前已经随机过且大于阈值，固定返回不健康状态
			status = "RESOURCE_EXHAUSTED"
			statusCode = http.StatusBadRequest
			message = fmt.Sprintf("资源使用率过高: 内存使用率 %.2f%% (阈值: %.2f%%), CPU使用率 %.2f%% (阈值: %.2f%%)",
				memUsagePercent, h.Config.ResourceThresholdMemoryPercent,
				cpuUsagePercent, h.Config.ResourceThresholdCPUPercent)
			log.WithFields(logrus.Fields{
				"status":  status,
				"message": message,
			}).Info("健康检查结果: 之前已决策固定拒绝流量")
		}
	} else {
		// 资源未过载，重置标志位，允许下次资源过载时重新随机
		shouldRandomize = true

		// 资源未过载
		message = fmt.Sprintf("健康检查通过: 内存使用率 %.2f%%, CPU使用率 %.2f%%, Pod可用率 %.2f%%",
			memUsagePercent, cpuUsagePercent, podsRatio)
	}

	details["status"] = status
	details["message"] = message
	w.WriteHeader(statusCode)
	h.writeJSONResponse(w, details)
	log.WithFields(logrus.Fields{
		"status":   status,
		"message":  message,
		"duration": time.Since(startTime),
	}).Info("健康检查请求处理完成")
}

// 收集资源指标
func (h *HealthHandler) collectResourceMetrics() (*metrics.ResourceMetrics, error) {
	log.Info("开始收集资源指标")
	startTime := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	metrics := &metrics.ResourceMetrics{
		ContainerName: h.Config.ContainerName,
	}

	// 获取Deployment信息
	log.WithField("deployment", h.Config.DeploymentName).Info("获取Deployment信息")
	deploymentInfo, err := h.K8sClient.GetDeploymentInfo(ctx)
	if err != nil {
		log.WithError(err).Error("获取Deployment信息失败")
	} else if deploymentInfo != nil {
		metrics.DeploymentReplicas = deploymentInfo.Replicas
		metrics.DeploymentAvailableReplicas = deploymentInfo.AvailableReplicas
		log.WithFields(logrus.Fields{
			"replicas":           deploymentInfo.Replicas,
			"available_replicas": deploymentInfo.AvailableReplicas,
		}).Debug("Deployment信息")
	}

	// 获取容器资源限制 (直接使用已缓存的值)
	log.WithField("container", h.Config.ContainerName).Info("获取容器资源限制")
	containerLimits, err := h.K8sClient.GetContainerLimits(ctx)
	if err != nil {
		log.WithError(err).Error("获取容器资源限制失败")
		return nil, fmt.Errorf("无法获取容器资源限制: %v", err)
	}
	metrics.ContainerCPULimit = containerLimits.CPULimit
	metrics.ContainerMemLimit = containerLimits.MemLimit
	log.WithFields(logrus.Fields{
		"cpu":    containerLimits.CPULimit,
		"memory": containerLimits.MemLimit,
	}).Debug("容器资源限制")

	// 获取Pod信息
	log.WithField("pod", h.Config.PodName).Info("获取Pod信息")
	podInfo, err := h.K8sClient.GetPodInfo(ctx)
	if err != nil {
		log.WithError(err).Error("获取Pod信息失败")
	} else if podInfo != nil && podInfo.Containers != nil {
		container := podInfo.Containers[h.Config.ContainerName]
		if container != nil {
			metrics.ContainerReady = container.Ready
			log.WithField("ready", container.Ready).Debug("容器就绪状态")
		}
	}

	// 获取Pod度量指标
	log.Info("获取Pod度量指标")
	podMetrics, err := h.MetricsCollector.GetPodMetrics(ctx)
	if err != nil {
		log.WithError(err).Error("获取Pod度量指标失败")
	} else if podMetrics != nil && podMetrics.Containers != nil {
		container := podMetrics.Containers[h.Config.ContainerName]
		if container != nil {
			metrics.ContainerCPUUsage = container.CPUUsage
			metrics.ContainerMemUsage = container.MemUsage
			log.WithFields(logrus.Fields{
				"cpu":    container.CPUUsage,
				"memory": container.MemUsage,
			}).Debug("容器资源使用")
		}
	}

	log.WithField("duration", time.Since(startTime)).Info("资源指标收集完成")
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
		log.WithError(err).Error("序列化JSON失败")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(jsonData)
}
