# Metrics Sidecar

## 项目背景

在高负载的Kubernetes环境中，我们面临着一个普遍而棘手的挑战：**如何智能地管理资源过载Pod的流量？** 当某个Pod的资源使用率飙升时，继续向其分发请求往往会导致服务质量严重下降，甚至引发连锁故障。传统的Kubernetes机制在这方面存在明显不足。

**⚠️ 现实痛点**

- 🔥 **资源热点问题**：高负载Pod继续接收流量，导致响应时间剧增，用户体验迅速恶化
- ⏱️ **扩容滞后效应**：HPA虽能水平扩展资源，但难以即时解决已过载实例的问题
- 🔄 **复杂解决方案**：服务网格等高级方案虽有效，但引入过多复杂性和资源开销

**🚀 创新解决方案**  
Metrics Sidecar 提供了一种优雅且轻量级的解决方案，它巧妙地结合了Kubernetes原生能力与资源感知逻辑：

- 利用Kubernetes的`readinessProbe`机制实现无侵入式流量控制
- 通过metrics-server实时监控容器资源使用情况
- 基于智能算法和随机退避策略确保服务弹性和可用性
- 零代码修改，作为Sidecar容器即插即用

这一设计使应用程序能够自动对资源压力做出反应，在资源紧张时主动减少流量负担，实现真正的资源自适应流量管理。

---

Kubernetes指标收集工具，用于监控Pod和容器的资源使用情况。适合作为sidecar容器部署，提供资源监控和健康检查接口。

## 🔍 核心功能

- 📊 **资源智能监控**：通过metrics-server API实时追踪Pod和容器的CPU、内存使用状况
- 🚦 **自适应健康检查**：提供智能`/healthz`接口，根据资源使用率动态调整Pod可用性
- 🎛️ **灵活阈值配置**：支持通过环境变量精细调整资源阈值和服务保护策略
- 🔧 **多场景适配**：支持通过环境变量或配置文件灵活定制各种运行参数
- 🌐 **双模式部署**：同时支持Kubernetes集群内（InCluster）和集群外的运行环境

## ⚙️ 工作原理

1. 🔄 作为sidecar容器与主应用部署在同一Pod中
2. 📈 持续监控并收集目标容器的资源使用指标
3. 🚦 通过`/healthz`接口智能控制流量：
   - 当CPU和内存同时超过配置阈值时，启动随机退避机制
   - 随机退避确保最少有`MINIMUM_PODS_TO_KEEP_PERCENT`比例的Pod保持服务
   - 资源使用正常或Pod可用率低于保护阈值时保持服务可用
4. 📋 通过`/metrics`接口提供完整的资源使用详情，便于监控和分析

## 🔄 随机退避机制

为避免资源过载时服务完全不可用，Metrics Sidecar实现了智能的随机退避策略：

1. 🔍 **资源监测**：当检测到容器CPU和内存同时超过预设阈值时触发
2. 🎲 **随机决策**：系统为每个Pod生成0-100之间的随机值
3. 🧮 **智能比较**：将随机值与`MINIMUM_PODS_TO_KEEP_PERCENT`进行对比
   - 随机值 > 阈值：Pod标记为不健康，暂时拒绝新流量
   - 随机值 ≤ 阈值：Pod保持健康状态，继续服务
4. 🛡️ **服务保护**：确保即使在极端压力下，服务整体仍然可用
5. 🌊 **流量平滑**：随着Pod重启或资源释放，系统自动恢复平衡状态

这种机制特别适合处理流量突增、资源紧张的场景，通过牺牲部分实例的可用性来保障整体服务的稳定性和响应速度。

## 🏗️ 项目结构

项目采用规范的Go模块化架构设计：

```
├── cmd/                      # 命令行程序入口
│   └── metrics-sidecar/      # 主应用入口点
│       └── main.go           # 程序主入口
├── pkg/                      # 核心功能模块
│   ├── config/               # 配置管理模块
│   ├── handlers/             # HTTP处理器模块
│   ├── k8s/                  # Kubernetes客户端
│   ├── logger/               # 日志系统模块
│   └── metrics/              # 指标收集与处理
├── kubernetes/               # K8s部署配置
│   └── cluster-rbac.yaml     # 集群级权限配置
├── Dockerfile                # 容器构建定义
├── go.mod                    # Go模块依赖
└── README.md                 # 项目文档
```

## 📝 日志系统

项目采用结构化的日志系统，基于logrus库实现，支持不同日志级别和上下文信息的记录。

### 日志级别

通过环境变量`LOG_LEVEL`可配置不同的日志级别：

| 级别 | 描述 | 使用场景 |
|:----:|:-----|:--------|
| `debug` | 详细的调试信息 | 开发环境，问题排查 |
| `info` | 常规运行信息 | 生产环境默认级别 |
| `warn` | 警告信息 | 潜在问题，但不影响正常运行 |
| `error` | 错误信息 | 影响功能的问题 |

### 日志格式

日志输出包含以下信息：
- 时间戳：精确到毫秒
- 日志级别：颜色编码区分不同级别
- 组件名称：指示日志来源模块
- 调用位置：文件名和行号
- 上下文信息：结构化字段
- 日志消息：实际内容

### 使用示例

每个模块使用特定的logger实例，自动添加组件标识：

```go
// 获取特定组件的logger
log := logger.GetLogger("health")

// 基本信息日志
log.Info("系统启动中...")

// 带字段的结构化日志
log.WithFields(logrus.Fields{
    "pod": podName,
    "namespace": namespace,
}).Info("资源监控开始")

// 错误日志
log.WithError(err).Error("连接失败")
```

### HTTP请求日志

所有HTTP请求自动记录以下信息：
- 远程地址：客户端IP
- 请求方法：GET, POST等
- 请求路径：API路径
- 处理时间：请求处理耗时

## 🚀 构建与运行

### 本地开发环境

```bash
# 构建应用
go build -o metrics-sidecar ./cmd/metrics-sidecar

# 本地运行（需要kubeconfig文件）
./metrics-sidecar
```

### 容器环境

```bash
# 构建Docker镜像
docker build -t metrics-sidecar:latest .

# 外部模式运行容器
docker run -v $HOME/.kube/config:/app/kube-config.yaml metrics-sidecar:latest
```

## 📋 前置依赖

- ✅ Kubernetes集群已安装metrics-server组件
- ✅ 目标容器已设置资源限制（resources.limits）
- ✅ 已配置适当的RBAC权限（详见下文）

### metrics-server安装指南

<details>
<summary>展开查看安装方法</summary>

1. **使用Helm安装**:

   ```bash
   helm repo add metrics-server https://kubernetes-sigs.github.io/metrics-server/
   helm upgrade --install metrics-server metrics-server/metrics-server --namespace kube-system
   ```

2. **使用YAML文件安装**:

   ```bash
   kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
   ```

3. **Minikube环境**:

   ```bash
   minikube addons enable metrics-server
   ```

</details>

## 🔌 Kubernetes集成

在Kubernetes中，通过以下配置将Metrics Sidecar添加到现有部署中：

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
        - name: LOG_LEVEL
          value: "info"  # 可选值: debug, info, warn, error
        
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

## ⚙️ 配置参数

通过环境变量可灵活配置Metrics Sidecar的行为:

| 参数名称 | 描述 | 默认值 |
|:-------:|:-----|:-----:|
| `NAMESPACE` | Kubernetes命名空间 | test-sp |
| `DEPLOYMENT_NAME` | 要监控的Deployment名称 | aliexpress |
| `CONTAINER_NAME` | 要监控的容器名称 | aliexpress |
| `POD_NAME` | 要监控的Pod名称 | aliexpress-6c7687ddb-gh5mb |
| `RESOURCE_THRESHOLD_MEMORY_PERCENT` | 内存使用率告警阈值(%) | 80 |
| `RESOURCE_THRESHOLD_CPU_PERCENT` | CPU使用率告警阈值(%) | 80 |
| `MINIMUM_PODS_TO_KEEP_PERCENT` | 最小可用Pod百分比和随机退避阈值(%) | 50 |
| `HTTP_PORT` | HTTP服务监听端口 | 8333 |
| `LOG_LEVEL` | 日志级别，支持debug/info/warn/error | info |

### 🔄 MINIMUM_PODS_TO_KEEP_PERCENT参数详解

`MINIMUM_PODS_TO_KEEP_PERCENT`参数具有双重功能：

1. **🛡️ 最小可用Pod保护**：当部署中可用Pod百分比低于此值时，即使资源过载，也会保持Pod健康状态
2. **⚖️ 随机退避阈值**：资源过载时，约有此百分比的Pod会保持健康状态继续服务

例如，设置为30%意味着：

- 当部署中可用Pod少于总数30%时，所有Pod都会保持健康状态
- 资源过载时，大约30%的Pod会继续接收流量，其余70%将暂时拒绝新请求

### 📊 LOG_LEVEL参数详解

`LOG_LEVEL`参数控制日志输出的详细程度：

- **debug**: 输出所有级别日志，包括详细的调试信息，适用于问题排查和开发环境
- **info**: 输出信息、警告和错误日志，适用于正常运行的生产环境（默认）
- **warn**: 仅输出警告和错误信息，减少日志量
- **error**: 仅输出错误信息，最小化日志输出

在生产环境中，建议使用`info`级别以保持合理的日志详细度；在排查问题时可临时调整为`debug`级别获取更多信息。

## 🧪 单元测试

项目包含全面的单元测试套件，确保核心功能稳定可靠。

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

# 生成详细覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### 测试范围

✅ **配置管理**：验证环境变量解析和默认值机制  
✅ **指标收集**：测试资源指标的收集和处理逻辑  
✅ **健康检查**：验证各种资源使用场景下的健康状态判断  
✅ **计算函数**：测试资源比例和阈值计算的准确性  
✅ **日志系统**：验证不同日志级别的正确过滤和格式化输出

## 🔐 RBAC权限配置

Metrics Sidecar需要特定的Kubernetes权限才能访问Pod、Deployment信息和metrics-server数据。根据您的监控需求，可以选择两种权限配置方案：

<details>
<summary><b>💼 命名空间级别权限</b> (单一命名空间监控)</summary>

适用于只监控单个命名空间内的应用场景。通过命名空间级别的RBAC配置创建所需权限。

```bash
kubectl apply -f kubernetes/rbac.yaml
```

这将创建：

- **ServiceAccount**: `metrics-sidecar`（在当前命名空间中）
- **Role**: 具有访问特定命名空间内Pod、Deployment和Pod指标的权限
- **RoleBinding**: 将Role绑定到ServiceAccount

</details>

<details>
<summary><b>🌐 集群级别权限</b> (跨命名空间监控)</summary>

适用于需要监控多个命名空间的场景，特别是需要访问节点级指标时。创建并应用以下资源：

```bash
kubectl apply -f kubernetes/cluster-rbac.yaml
```

这将创建：

- **ServiceAccount**: `metrics-sidecar-cluster`（在default命名空间中）
- **ClusterRole**: 具有以下权限：
  - 访问所有命名空间中的Deployment资源
  - 访问所有命名空间中的Pod资源
  - 访问所有命名空间中的Pod和Node指标资源
- **ClusterRoleBinding**: 将ClusterRole绑定到ServiceAccount

</details>

### 在部署中指定服务账号

在您的Deployment配置中，根据您选择的RBAC配置指定相应的ServiceAccount：

```yaml
spec:
  template:
    spec:

      
      # 使用集群级别权限
      serviceAccountName: metrics-sidecar-cluster
```

### 常见权限问题

如果遇到类似`nodes.metrics.k8s.io is forbidden`的错误，这通常表示缺少对集群级别资源的访问权限。在这种情况下，您应该使用集群级别的权限配置（cluster-rbac.yaml）。

## 📊 探针机制优化

通过精细调整readinessProbe参数，可以实现更智能的流量控制：

| 参数 | 描述 | 推荐值 |
|:----:|:-----|:-----:|
| **initialDelaySeconds** | 容器启动后首次检查的等待时间 | 10-30秒 |
| **periodSeconds** | 执行检查的频率 | 10-15秒 |
| **timeoutSeconds** | 检查超时时间 | 3-5秒 |
| **failureThreshold** | 连续失败次数阈值 | 2-3次 |
| **successThreshold** | 恢复所需连续成功次数 | 1次 |

💡 **最佳实践**：适当增大`failureThreshold`可以避免因短暂资源波动导致的频繁流量切换，提高系统稳定性。

## 📝 注意事项

- ⚠️ 初始化时无法获取容器资源限制信息会导致程序退出
- 🔄 CPU和内存同时超过阈值才会触发随机退避机制
- 🛡️ 当可用Pod比例低于最小阈值时，所有Pod会保持健康状态
- 🔍 程序会自动检测运行环境，在K8s集群内部自动使用InCluster配置

## 📚 参考资料

- [Kubernetes官方文档 - Pod健康检查](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/)
- [metrics-server GitHub仓库](https://github.com/kubernetes-sigs/metrics-server)
- [Go client for Kubernetes](https://github.com/kubernetes/client-go)
