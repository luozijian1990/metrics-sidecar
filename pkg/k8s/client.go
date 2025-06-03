package k8s

import (
	"context"
	"fmt"
	"log"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"

	"metrics-sidecar/pkg/config"
	"metrics-sidecar/pkg/metrics"
)

// Client 封装Kubernetes相关客户端
type Client struct {
	KubeClient      *kubernetes.Clientset
	MetricsClient   *metricsclient.Clientset
	Config          *config.Config
	ContainerLimits *metrics.ContainerLimits // 存储容器资源限制
}

// NewClient 创建并返回一个新的Client
func NewClient(cfg *config.Config) (*Client, error) {
	var kubeConfig *rest.Config
	var err error

	if cfg.InClusterConfig {
		// 使用集群内配置
		log.Printf("使用InCluster配置连接Kubernetes集群")
		kubeConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("创建InCluster配置失败: %v", err)
		}
	} else {
		// 使用外部配置文件
		log.Printf("使用kubeconfig文件连接Kubernetes集群: %s", cfg.KubeconfigPath)
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", cfg.KubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("构建K8s配置失败: %v", err)
		}
	}

	// 创建Kubernetes客户端
	clientSet, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("创建Kubernetes客户端失败: %v", err)
	}

	// 创建Metrics客户端
	metricsConfig := *kubeConfig
	metricsConfig.GroupVersion = &schema.GroupVersion{
		Group:   "metrics.k8s.io",
		Version: "v1beta1",
	}

	metricsClient, err := metricsclient.NewForConfig(&metricsConfig)
	if err != nil {
		return nil, fmt.Errorf("创建Metrics客户端失败: %v", err)
	}

	client := &Client{
		KubeClient:    clientSet,
		MetricsClient: metricsClient,
		Config:        cfg,
	}

	// 初始化时立即获取容器资源限制
	containerLimits, err := client.initContainerLimits()
	if err != nil {
		return nil, fmt.Errorf("获取容器资源限制失败: %v", err)
	}
	client.ContainerLimits = containerLimits

	return client, nil
}

// initContainerLimits 初始化时获取容器资源限制
func (c *Client) initContainerLimits() (*metrics.ContainerLimits, error) {
	log.Printf("初始化: 获取容器[%s]资源限制", c.Config.ContainerName)

	// 设置超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 重试机制
	maxRetries := 5
	retryInterval := 3 * time.Second

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			log.Printf("尝试获取容器资源限制, 第%d次重试...", i+1)
			time.Sleep(retryInterval)
		}

		deploy, err := c.KubeClient.AppsV1().Deployments(c.Config.Namespace).Get(ctx, c.Config.DeploymentName, metav1.GetOptions{})
		if err != nil {
			lastErr = fmt.Errorf("获取Deployment失败: %v", err)
			log.Printf("获取Deployment信息失败: %v, 将重试", err)
			continue
		}

		// 查找容器并获取资源限制
		for _, container := range deploy.Spec.Template.Spec.Containers {
			if container.Name == c.Config.ContainerName {
				cpuLimit := container.Resources.Limits.Cpu().MilliValue()
				memLimit := container.Resources.Limits.Memory().Value() / (1024 * 1024)

				// 验证资源限制是否有效
				if cpuLimit <= 0 || memLimit <= 0 {
					lastErr = fmt.Errorf("无效的资源限制值: CPU=%dm, Memory=%dMi", cpuLimit, memLimit)
					log.Printf("警告: %v", lastErr)
					continue
				}

				log.Printf("成功获取容器资源限制: CPU=%dm, Memory=%dMi", cpuLimit, memLimit)
				return &metrics.ContainerLimits{
					CPULimit: cpuLimit,
					MemLimit: memLimit,
				}, nil
			}
		}

		lastErr = fmt.Errorf("在Deployment[%s]中未找到容器[%s]", c.Config.DeploymentName, c.Config.ContainerName)
		log.Printf("警告: %v", lastErr)
	}

	// 所有重试都失败了
	return nil, fmt.Errorf("经过%d次尝试，无法获取容器资源限制: %v", maxRetries, lastErr)
}

// GetDeploymentInfo 获取Deployment信息
func (c *Client) GetDeploymentInfo(ctx context.Context) (*metrics.DeploymentMetrics, error) {
	deploy, err := c.KubeClient.AppsV1().Deployments(c.Config.Namespace).Get(ctx, c.Config.DeploymentName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("获取Deployment失败: %v", err)
	}

	return &metrics.DeploymentMetrics{
		Name:              deploy.Name,
		Namespace:         deploy.Namespace,
		Replicas:          deploy.Status.Replicas,
		AvailableReplicas: deploy.Status.AvailableReplicas,
	}, nil
}

// GetContainerLimits 获取容器资源限制 (现在直接返回初始化时存储的值)
func (c *Client) GetContainerLimits(ctx context.Context) (*metrics.ContainerLimits, error) {
	if c.ContainerLimits == nil {
		return nil, fmt.Errorf("容器资源限制未初始化")
	}
	return c.ContainerLimits, nil
}

// GetPodInfo 获取Pod信息
func (c *Client) GetPodInfo(ctx context.Context) (*metrics.PodMetrics, error) {
	pod, err := c.KubeClient.CoreV1().Pods(c.Config.Namespace).Get(ctx, c.Config.PodName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("获取Pod失败: %v", err)
	}

	result := &metrics.PodMetrics{
		Name:       pod.Name,
		Namespace:  pod.Namespace,
		Containers: make(map[string]*metrics.ContainerMetrics),
	}

	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name == c.Config.ContainerName {
			result.Containers[containerStatus.Name] = &metrics.ContainerMetrics{
				Name:  containerStatus.Name,
				Ready: containerStatus.Ready,
			}
		}
	}

	return result, nil
}
