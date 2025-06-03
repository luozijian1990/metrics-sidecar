# Metrics Sidecar

## 项目背景

在Kubernetes环境中运行无状态服务时，我们经常遇到这样的挑战：当某个Pod资源使用率过高时，如果继续接收流量，可能会导致服务响应缓慢甚至崩溃。然而，Kubernetes原生并没有提供基于资源使用率自动调节流量的机制。

**常见问题**：
- 资源使用率过高的Pod继续接收流量，导致用户体验下降
- 现有的HPA机制只能水平扩展，但无法立即解决已经过载的Pod问题
- 服务网格等解决方案引入了较大的复杂性和额外开销

**解决方案**：  
Metrics Sidecar利用Kubernetes的readinessProbe机制，通过metrics-server查询pod主容器实际资源使用情况，基于容器的实际资源使用情况动态调整Pod是否接收流量，在不修改应用代码的情况下实现资源自适应的流量控制。

---

Kubernetes指标收集工具，用于监控Pod和容器的资源使用情况。适合作为sidecar容器部署，提供资源监控和健康检查接口。

## 主要功能

- **资源监控**：通过metrics-server API监控Pod和容器的CPU、内存使用情况
- **健康检查**：提供HTTP接口（`/healthz`）基于资源使用率进行健康状态检查
- **弹性阈值**：支持通过环境变量配置资源使用阈值和最小可用Pod比例
- **配置灵活**：支持通过环境变量或配置文件进行定制
- **双模式运行**：支持在Kubernetes集群内（InCluster）和集群外运行

## 工作原理

1. 作为sidecar容器部署到现有Pod中
2. 定期收集容器的资源使用情况
3. 通过`/healthz`接口暴露健康状态：
   - 当CPU和内存同时超过配置阈值时返回`400`状态码
   - 资源使用正常或Pod可用率低于配置阈值时返回`200`状态码
4. 通过`/metrics`接口提供详细的资源使用数据

## 依赖条件

- Kubernetes集群需安装metrics-server
- 目标容器必须设置资源限制（resources.limits）
- 需要配置适当的RBAC权限

### metrics-server安装方法

1. 使用Helm安装:
   ```bash
   helm repo add metrics-server https://kubernetes-sigs.github.io/metrics-server/
   helm upgrade --install metrics-server metrics-server/metrics-server --namespace kube-system
   ```

2. 使用YAML文件安装:
   ```bash
   kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
   ```

3. 如果是Minikube，可以使用插件启用:
   ```bash
   minikube addons enable metrics-server
   ```

## Kubernetes集成

在Kubernetes中，您可以将此sidecar添加到现有部署中，并配置探针以实现自动流量控制：

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      # 指定ServiceAccount
      serviceAccountName: metrics-sidecar
      containers:
      - name: main-app
        # 主应用容器配置...
        
      - name: metrics-sidecar
        image: metrics-sidecar:latest
        ports:
        - containerPort: 8333
        env:
        - name: NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: DEPLOYMENT_NAME
          value: "your-deployment-name"
        - name: CONTAINER_NAME
          value: "main-app"
        - name: RESOURCE_THRESHOLD_MEMORY_PERCENT
          value: "80"
        - name: RESOURCE_THRESHOLD_CPU_PERCENT
          value: "80"
        - name: MINIMUM_PODS_TO_KEEP_PERCENT
          value: "30"
        
      # 配置主应用的就绪探针指向sidecar的健康检查接口
      readinessProbe:
        httpGet:
          path: /healthz
          port: 8333
        initialDelaySeconds: 10
        periodSeconds: 15
        timeoutSeconds: 5
        failureThreshold: 2
        successThreshold: 1
```

## RBAC权限配置

Metrics Sidecar需要特定的Kubernetes权限才能访问Pod、Deployment信息和metrics-server数据。项目提供了两种RBAC配置方案：

### 命名空间级别权限

适用于只监控单个命名空间内的应用场景。创建并应用以下资源：

```bash
kubectl apply -f kubernetes/rbac.yaml
```

这将创建：
- ServiceAccount: `metrics-sidecar`
- Role: 具有访问特定命名空间内资源的权限
- RoleBinding: 将Role绑定到ServiceAccount

### 集群级别权限

适用于需要监控多个命名空间的场景。创建并应用以下资源：

```bash
kubectl apply -f kubernetes/cluster-rbac.yaml
```

这将创建：
- ServiceAccount: `metrics-sidecar-cluster`（在monitoring命名空间中）
- ClusterRole: 具有访问所有命名空间资源的权限
- ClusterRoleBinding: 将ClusterRole绑定到ServiceAccount

记得在部署时指定正确的ServiceAccount：

```yaml
spec:
  template:
    spec:
      serviceAccountName: metrics-sidecar  # 或 metrics-sidecar-cluster
```

## 探针机制说明

- **initialDelaySeconds**: 容器启动后首次进行检查的等待时间
- **periodSeconds**: 执行检查的频率
- **timeoutSeconds**: 检查超时时间
- **failureThreshold**: 被视为不健康前必须连续失败的次数（可作为缓冲机制）
- **successThreshold**: 从不健康状态恢复需要的连续成功次数

通过调整这些参数，可以实现缓冲机制，避免因短暂的资源峰值导致服务中断。

## 环境变量配置

可以通过环境变量来配置应用的行为。在Kubernetes中，通常通过Pod规范中的`env`字段来设置这些变量。

## 可配置的环境变量

| 变量名 | 描述 | 默认值 |
|-------|------|-------|
| NAMESPACE | Kubernetes命名空间 | test-sp |
| DEPLOYMENT_NAME | 要监控的Deployment名称 | aliexpress |
| CONTAINER_NAME | 要监控的容器名称 | aliexpress |
| POD_NAME | 要监控的Pod名称 | aliexpress-6c7687ddb-gh5mb |
| RESOURCE_THRESHOLD_MEMORY_PERCENT | 内存使用率阈值(%) | 80 |
| RESOURCE_THRESHOLD_CPU_PERCENT | CPU使用率阈值(%) | 80 |
| MINIMUM_PODS_TO_KEEP_PERCENT | 最小可用Pod百分比 | 50 |
| HTTP_PORT | HTTP服务端口 | 8333 |

## 单元测试

项目包含完整的单元测试，确保核心功能的正确性和稳定性。

### 运行测试

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./pkg/config/
go test ./pkg/metrics/
go test ./pkg/handlers/

# 查看测试覆盖率
go test -cover ./...

# 生成测试覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### 测试内容

项目单元测试覆盖以下关键功能：

- **配置管理**：测试环境变量解析和默认值处理
- **指标收集**：测试Pod和容器资源指标的收集
- **健康检查逻辑**：测试各种情况下的健康状态计算和HTTP响应
- **计算函数**：测试内存、CPU使用率和Pod比例的计算函数

## 注意事项

- 初始化时无法获取容器资源限制信息会导致程序退出
- 健康检查中，CPU和内存同时超过阈值才会返回不健康状态
- 当可用Pod比例低于最小阈值时，即使资源超限也不会返回不健康状态
- 程序会自动检测运行环境，在Kubernetes集群内部会使用InCluster配置模式

## 运行

### 本地开发

在本地开发时，需要提供kubeconfig文件：

```bash
# 默认从当前目录的kube-config.yaml加载配置
go run main.go

# 或者指定kubeconfig路径
go run main.go --kubeconfig=/path/to/kubeconfig
```

### 在Kubernetes中运行

在Kubernetes中运行时，程序会自动使用InCluster配置模式，无需指定kubeconfig。

### 容器构建

```bash
docker build -t metrics-sidecar:latest .
``` 