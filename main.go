package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"metrics-sidecar/pkg/config"
	"metrics-sidecar/pkg/handlers"
	"metrics-sidecar/pkg/k8s"
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
	log.Printf("%s %s %s - 完成耗时: %v",
		r.RemoteAddr, r.Method, r.URL.Path, time.Since(start))
}

// 检查metrics.k8s.io API是否可用
func checkMetricsAPIAvailability(k8sClient *k8s.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Printf("正在检查metrics.k8s.io API是否可用...")

	// 尝试列出所有节点的指标，验证API是否可用
	_, err := k8sClient.MetricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("metrics.k8s.io API不可用: %v", err)
	}

	log.Printf("metrics.k8s.io API检查通过")
	return nil
}

func main() {
	// 配置日志格式
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.Printf("指标采集服务启动中...")

	// 创建配置
	cfg := config.NewConfig()

	// 显示连接模式和配置信息
	if cfg.InClusterConfig {
		log.Printf("使用集群内配置模式 (InCluster)")
	} else {
		log.Printf("使用外部配置文件模式: %s", cfg.KubeconfigPath)
	}

	log.Printf("加载配置完成: Namespace=%s, DeploymentName=%s, ContainerName=%s",
		cfg.Namespace, cfg.DeploymentName, cfg.ContainerName)

	// 创建K8s客户端
	log.Printf("正在创建Kubernetes客户端...")
	k8sClient, err := k8s.NewClient(cfg)
	if err != nil {
		// 严重错误：在初始化时无法获取容器资源限制会导致panic
		log.Fatalf("致命错误: %v", err)
	}
	log.Printf("Kubernetes客户端创建成功")

	// 检查metrics.k8s.io API是否可用
	if err := checkMetricsAPIAvailability(k8sClient); err != nil {
		log.Printf("警告: %v", err)
		log.Fatalf("metrics-server未安装或不可用，程序无法继续运行")
	}

	// 创建度量指标收集器
	log.Printf("正在创建指标收集器...")
	metricsCollector := metrics.NewMetricsCollector(k8sClient.MetricsClient, cfg)
	log.Printf("指标收集器创建成功")

	// 创建HTTP处理器
	log.Printf("正在创建HTTP处理器...")
	healthHandler := handlers.NewHealthHandler(k8sClient, metricsCollector, cfg)
	metricsHandler := handlers.NewMetricsHandler(k8sClient, metricsCollector, cfg, healthHandler)
	log.Printf("HTTP处理器创建成功")

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

	// 启动HTTP服务器
	log.Printf("启动HTTP服务器，监听端口: %s", cfg.HttpPort)
	server := &http.Server{
		Addr:         ":" + cfg.HttpPort,
		Handler:      loggedRouter,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("指标采集服务已启动，等待请求...")
	log.Fatal(server.ListenAndServe())
}
