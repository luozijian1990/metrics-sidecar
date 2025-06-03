package config

import (
	"flag"
	"os"
	"path/filepath"
	"strconv"
)

// Config 保存应用程序配置
type Config struct {
	// K8s配置
	KubeconfigPath  string // kubeconfig文件路径
	InClusterConfig bool   // 是否使用InCluster配置
	Namespace       string // 命名空间
	DeploymentName  string // Deployment名称
	ContainerName   string // 容器名称
	PodName         string // Pod名称

	// 资源阈值配置
	ResourceThresholdMemoryPercent float64 // 内存使用率阈值百分比
	ResourceThresholdCPUPercent    float64 // CPU使用率阈值百分比
	MinimumPodsToKeepPercent       float64 // 最小可用Pod百分比

	// HTTP服务配置
	HttpPort string // HTTP服务端口
}

// 获取环境变量，如果不存在则返回默认值
func getEnvWithDefault(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// 获取环境变量并转换为浮点数
func getEnvAsFloat(key string, defaultValue float64) float64 {
	if value, exists := os.LookupEnv(key); exists {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

// 检测是否在Kubernetes集群内运行
func isRunningInCluster() bool {
	// 检查Pod服务账号令牌文件是否存在
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); !os.IsNotExist(err) {
		return true
	}
	return false
}

// NewConfig 创建配置实例
func NewConfig() *Config {
	// 检查是否在集群内运行
	inCluster := isRunningInCluster()

	var kubeconfig *string

	if inCluster {
		// 在集群内运行时不需要指定kubeconfig
		kubeconfig = flag.String("kubeconfig", "", "在集群内运行时忽略此参数")
	} else if home := os.Getenv("HOME"); home != "" {
		// 在集群外运行时，默认尝试使用当前目录下的kube-config.yaml
		defaultPath := filepath.Join(".", "kube-config.yaml")
		kubeconfig = flag.String("kubeconfig", defaultPath, "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

	flag.Parse()

	return &Config{
		KubeconfigPath:                 *kubeconfig,
		InClusterConfig:                inCluster,
		Namespace:                      getEnvWithDefault("NAMESPACE", "test-sp"),
		DeploymentName:                 getEnvWithDefault("DEPLOYMENT_NAME", "aliexpress"),
		ContainerName:                  getEnvWithDefault("CONTAINER_NAME", "aliexpress"),
		PodName:                        getEnvWithDefault("POD_NAME", "aliexpress-6c7687ddb-gh5mb"),
		ResourceThresholdMemoryPercent: getEnvAsFloat("RESOURCE_THRESHOLD_MEMORY_PERCENT", 80.0),
		ResourceThresholdCPUPercent:    getEnvAsFloat("RESOURCE_THRESHOLD_CPU_PERCENT", 80.0),
		MinimumPodsToKeepPercent:       getEnvAsFloat("MINIMUM_PODS_TO_KEEP_PERCENT", 50.0),
		HttpPort:                       getEnvWithDefault("HTTP_PORT", "8333"),
	}
}
