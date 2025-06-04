package logger

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"

	"metrics-sidecar/pkg/config"
)

// 初始化 logrus 日志配置
func Setup(cfg *config.Config) {
	// 设置日志格式
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			filename := path.Base(f.File)
			return "", fmt.Sprintf("%s:%d", filename, f.Line)
		},
	})

	// 设置输出到标准输出
	logrus.SetOutput(os.Stdout)

	// 设置日志级别
	logLevel := strings.ToLower(cfg.LogLevel)
	switch logLevel {
	case "debug":
		logrus.SetLevel(logrus.DebugLevel)
	case "info":
		logrus.SetLevel(logrus.InfoLevel)
	case "warn", "warning":
		logrus.SetLevel(logrus.WarnLevel)
	case "error":
		logrus.SetLevel(logrus.ErrorLevel)
	default:
		logrus.SetLevel(logrus.InfoLevel)
	}

	// 显示调用者信息
	logrus.SetReportCaller(true)

	logrus.WithFields(logrus.Fields{
		"level": logrus.GetLevel().String(),
	}).Info("日志系统初始化完成")
}

// GetLogger 返回一个预设了特定字段的 logger 实例
func GetLogger(component string) *logrus.Entry {
	return logrus.WithField("component", component)
}

// 以下是一些便捷的日志工具函数

// HTTPRequestCompleted 记录 HTTP 请求完成的日志
func HTTPRequestCompleted(method, path string, remoteAddr string, duration interface{}) {
	logrus.WithFields(logrus.Fields{
		"remote_addr": remoteAddr,
		"method":      method,
		"path":        path,
		"duration":    duration,
	}).Info("HTTP请求完成")
}

// StartupInfo 记录服务启动信息
func StartupInfo(config *config.Config) {
	logrus.Info("指标采集服务启动中...")

	// 显示连接模式和配置信息
	if config.InClusterConfig {
		logrus.Info("使用集群内配置模式 (InCluster)")
	} else {
		logrus.WithField("kubeconfig", config.KubeconfigPath).
			Info("使用外部配置文件模式")
	}

	logrus.WithFields(logrus.Fields{
		"namespace":       config.Namespace,
		"deployment_name": config.DeploymentName,
		"container_name":  config.ContainerName,
	}).Info("加载配置完成")
}

// ShutdownInfo 记录服务关闭信息
func ShutdownInfo(msg string) {
	logrus.Info(msg)
}

// Fatal 记录致命错误并退出程序
func Fatal(err error, msg string) {
	logrus.WithError(err).Fatal(msg)
}
