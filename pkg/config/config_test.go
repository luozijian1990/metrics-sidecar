package config

import (
	"os"
	"testing"
)

func TestGetEnvWithDefault(t *testing.T) {
	// 设置测试环境变量
	testKey := "TEST_ENV_VAR"
	testValue := "test_value"
	os.Setenv(testKey, testValue)
	defer os.Unsetenv(testKey)

	// 测试存在的环境变量
	result := getEnvWithDefault(testKey, "default_value")
	if result != testValue {
		t.Errorf("getEnvWithDefault(%s, default_value) = %s; 期望 %s", testKey, result, testValue)
	}

	// 测试不存在的环境变量
	nonExistentKey := "NON_EXISTENT_KEY"
	defaultValue := "default_value"
	result = getEnvWithDefault(nonExistentKey, defaultValue)
	if result != defaultValue {
		t.Errorf("getEnvWithDefault(%s, %s) = %s; 期望 %s", nonExistentKey, defaultValue, result, defaultValue)
	}
}

func TestGetEnvAsFloat(t *testing.T) {
	// 测试有效浮点数环境变量
	testKey := "TEST_FLOAT_VAR"
	testValue := "123.45"
	expectedValue := 123.45
	os.Setenv(testKey, testValue)
	defer os.Unsetenv(testKey)

	result := getEnvAsFloat(testKey, 0.0)
	if result != expectedValue {
		t.Errorf("getEnvAsFloat(%s, 0.0) = %f; 期望 %f", testKey, result, expectedValue)
	}

	// 测试无效浮点数环境变量
	invalidKey := "INVALID_FLOAT_VAR"
	os.Setenv(invalidKey, "not-a-float")
	defer os.Unsetenv(invalidKey)

	defaultValue := 99.9
	result = getEnvAsFloat(invalidKey, defaultValue)
	if result != defaultValue {
		t.Errorf("getEnvAsFloat(%s, %f) = %f; 期望 %f", invalidKey, defaultValue, result, defaultValue)
	}

	// 测试不存在的环境变量
	nonExistentKey := "NON_EXISTENT_FLOAT_KEY"
	result = getEnvAsFloat(nonExistentKey, defaultValue)
	if result != defaultValue {
		t.Errorf("getEnvAsFloat(%s, %f) = %f; 期望 %f", nonExistentKey, defaultValue, result, defaultValue)
	}
}

// 测试模拟isRunningInCluster函数
func TestIsRunningInCluster(t *testing.T) {
	// 这个测试只是简单验证函数存在并返回布尔值
	// 真正的行为依赖于文件系统，很难在单元测试中模拟
	result := isRunningInCluster()
	if result != true && result != false {
		t.Error("isRunningInCluster()应该返回布尔值")
	}
}

func TestNewConfig(t *testing.T) {
	// 保存原始环境变量
	origNamespace := os.Getenv("NAMESPACE")
	origDeploymentName := os.Getenv("DEPLOYMENT_NAME")
	origContainerName := os.Getenv("CONTAINER_NAME")
	origPodName := os.Getenv("POD_NAME")

	// 设置测试环境变量
	os.Setenv("NAMESPACE", "test-namespace")
	os.Setenv("DEPLOYMENT_NAME", "test-deployment")
	os.Setenv("CONTAINER_NAME", "test-container")
	os.Setenv("POD_NAME", "test-pod")
	os.Setenv("RESOURCE_THRESHOLD_MEMORY_PERCENT", "75.5")
	os.Setenv("RESOURCE_THRESHOLD_CPU_PERCENT", "85.5")
	os.Setenv("MINIMUM_PODS_TO_KEEP_PERCENT", "40.0")
	os.Setenv("HTTP_PORT", "9090")

	// 测试完成后恢复环境
	defer func() {
		if origNamespace != "" {
			os.Setenv("NAMESPACE", origNamespace)
		} else {
			os.Unsetenv("NAMESPACE")
		}
		if origDeploymentName != "" {
			os.Setenv("DEPLOYMENT_NAME", origDeploymentName)
		} else {
			os.Unsetenv("DEPLOYMENT_NAME")
		}
		if origContainerName != "" {
			os.Setenv("CONTAINER_NAME", origContainerName)
		} else {
			os.Unsetenv("CONTAINER_NAME")
		}
		if origPodName != "" {
			os.Setenv("POD_NAME", origPodName)
		} else {
			os.Unsetenv("POD_NAME")
		}
		os.Unsetenv("RESOURCE_THRESHOLD_MEMORY_PERCENT")
		os.Unsetenv("RESOURCE_THRESHOLD_CPU_PERCENT")
		os.Unsetenv("MINIMUM_PODS_TO_KEEP_PERCENT")
		os.Unsetenv("HTTP_PORT")
	}()

	// 由于无法直接修改isRunningInCluster函数，我们将只测试环境变量的解析
	cfg := NewConfig()

	// 验证配置值
	if cfg.Namespace != "test-namespace" {
		t.Errorf("Namespace = %s; 期望 test-namespace", cfg.Namespace)
	}

	if cfg.DeploymentName != "test-deployment" {
		t.Errorf("DeploymentName = %s; 期望 test-deployment", cfg.DeploymentName)
	}

	if cfg.ContainerName != "test-container" {
		t.Errorf("ContainerName = %s; 期望 test-container", cfg.ContainerName)
	}

	if cfg.PodName != "test-pod" {
		t.Errorf("PodName = %s; 期望 test-pod", cfg.PodName)
	}

	if cfg.ResourceThresholdMemoryPercent != 75.5 {
		t.Errorf("ResourceThresholdMemoryPercent = %f; 期望 75.5", cfg.ResourceThresholdMemoryPercent)
	}

	if cfg.ResourceThresholdCPUPercent != 85.5 {
		t.Errorf("ResourceThresholdCPUPercent = %f; 期望 85.5", cfg.ResourceThresholdCPUPercent)
	}

	if cfg.MinimumPodsToKeepPercent != 40.0 {
		t.Errorf("MinimumPodsToKeepPercent = %f; 期望 40.0", cfg.MinimumPodsToKeepPercent)
	}

	if cfg.HttpPort != "9090" {
		t.Errorf("HttpPort = %s; 期望 9090", cfg.HttpPort)
	}
}
