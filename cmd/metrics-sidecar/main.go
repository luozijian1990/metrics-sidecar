package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"metrics-sidecar/pkg/config"
	"metrics-sidecar/pkg/handlers"
	"metrics-sidecar/pkg/k8s"
	"metrics-sidecar/pkg/logger"
	"metrics-sidecar/pkg/metrics"
)

// 自定义日志格式的HTTP服务器
type loggingHandler struct {
	handler http.Handler
}

func (h loggingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// 调用实际的处理器
	h.handler.ServeHTTP(w, r)

	// 记录请求完成后的日志
	logger.HTTPRequestCompleted(r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
}

// setupHTTPServer 配置HTTP服务器和路由
func setupHTTPServer(healthHandler http.Handler, metricsHandler http.Handler, port string) *http.Server {
	// 设置HTTP路由
	mux := http.NewServeMux()
	mux.Handle("/healthz", healthHandler)
	mux.Handle("/metrics", metricsHandler)

	// 添加首页路由，提供基本信息
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("指标采集服务正在运行\n\n可用接口:\n- /healthz: 健康检查\n- /metrics: 资源指标"))
	})

	// 创建带日志的HTTP服务器
	loggedRouter := loggingHandler{handler: mux}

	// 创建HTTP服务器
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      loggedRouter,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return server
}

// checkMetricsAPIAvailability 检查metrics.k8s.io API是否可用
func checkMetricsAPIAvailability(k8sClient *k8s.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log := logger.GetLogger("metrics-api")
	log.Info("正在检查metrics.k8s.io API是否可用...")

	// 尝试列出所有节点的指标，验证API是否可用
	_, err := k8sClient.MetricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("metrics.k8s.io API不可用: %v", err)
	}

	log.Info("metrics.k8s.io API检查通过")
	return nil
}

func main() {
	// 创建配置
	cfg := config.NewConfig()

	// 初始化日志系统
	logger.Setup(cfg)

	// 显示启动信息
	logger.StartupInfo(cfg)

	// 创建K8s客户端
	log := logger.GetLogger("main")
	log.Info("正在创建Kubernetes客户端...")
	k8sClient, err := k8s.NewClient(cfg)
	if err != nil {
		// 严重错误：在初始化时无法获取容器资源限制会导致panic
		logger.Fatal(err, "致命错误")
	}
	log.Info("Kubernetes客户端创建成功")

	// 检查metrics.k8s.io API是否可用
	if err := checkMetricsAPIAvailability(k8sClient); err != nil {
		logrus.WithError(err).Warn("警告")
		logger.Fatal(nil, "metrics-server未安装或不可用，程序无法继续运行")
	}

	// 创建度量指标收集器
	log.Info("正在创建指标收集器...")
	metricsCollector := metrics.NewMetricsCollector(k8sClient.MetricsClient, cfg)
	log.Info("指标收集器创建成功")

	// 创建HTTP处理器
	log.Info("正在创建HTTP处理器...")
	healthHandler := handlers.NewHealthHandler(k8sClient, metricsCollector, cfg)
	metricsHandler := handlers.NewMetricsHandler(k8sClient, metricsCollector, cfg, healthHandler)
	log.Info("HTTP处理器创建成功")

	// 设置HTTP服务器
	log.WithField("port", cfg.HttpPort).Info("启动HTTP服务器")
	server := setupHTTPServer(healthHandler, metricsHandler, cfg.HttpPort)

	// 创建信号监听通道
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// 在独立的goroutine中启动服务器
	go func() {
		log.Info("指标采集服务已启动，等待请求...")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal(err, "HTTP服务器错误")
		}
	}()

	// 等待终止信号
	<-stop
	logger.ShutdownInfo("收到终止信号，开始优雅关闭...")

	// 创建一个5秒超时的上下文用于优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 优雅地关闭服务器，等待活跃连接完成
	if err := server.Shutdown(ctx); err != nil {
		logger.Fatal(err, "服务器关闭错误")
	}

	logger.ShutdownInfo("服务器已安全关闭")
}
