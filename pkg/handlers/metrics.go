package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"metrics-sidecar/pkg/config"
	"metrics-sidecar/pkg/k8s"
	"metrics-sidecar/pkg/logger"
	"metrics-sidecar/pkg/metrics"

	"github.com/sirupsen/logrus"
)

var (
	// metrics处理器的日志器
	metricsLog = logger.GetLogger("metrics")
)

// MetricsHandler 指标API处理器
type MetricsHandler struct {
	K8sClient        *k8s.Client
	MetricsCollector *metrics.MetricsCollector
	Config           *config.Config
	HealthHandler    *HealthHandler
}

// NewMetricsHandler 创建新的指标处理器
func NewMetricsHandler(k8sClient *k8s.Client, metricsCollector *metrics.MetricsCollector, cfg *config.Config, healthHandler *HealthHandler) *MetricsHandler {
	return &MetricsHandler{
		K8sClient:        k8sClient,
		MetricsCollector: metricsCollector,
		Config:           cfg,
		HealthHandler:    healthHandler,
	}
}

// ServeHTTP 实现http.Handler接口
func (h *MetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	metricsLog.WithFields(logrus.Fields{
		"method": r.Method,
		"path":   r.URL.Path,
	}).Info("开始处理指标API请求")

	// 复用健康检查处理器的指标收集逻辑
	metrics, err := h.HealthHandler.collectResourceMetrics()
	if err != nil {
		metricsLog.WithError(err).Error("获取指标失败")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "Failed to collect metrics: %v", err)
		return
	}

	metricsLog.Info("成功收集到指标数据，正在返回JSON响应")
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(metrics)
	if err != nil {
		metricsLog.WithError(err).Error("序列化JSON失败")
	}

	metricsLog.WithField("duration", time.Since(startTime)).Info("指标API请求处理完成")
}
