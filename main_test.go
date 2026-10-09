package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"LISTEN_PORT", "SILICONFLOW_API_KEY", "SILICONFLOW_BASE_URL", "SILICONFLOW_MODEL"} {
		t.Setenv(name, "")
	}
	t.Setenv("SILICONFLOW_FALLBACK_MODELS", "")
}

func TestConfigEnvironmentOverridesFile(t *testing.T) {
	clearConfigEnv(t)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"listen_port":8066,"siliconflow":{"api_key":"file-key","model":"file-model","temperature":0.2}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LISTEN_PORT", "9000")
	t.Setenv("SILICONFLOW_API_KEY", "environment-key")
	t.Setenv("SILICONFLOW_MODEL", "environment-model")
	t.Setenv("SILICONFLOW_BASE_URL", "http://127.0.0.1/chat")
	t.Setenv("SILICONFLOW_FALLBACK_MODELS", " model-a, ,model-b ")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenPort != 9000 || cfg.SiliconFlow.APIKey != "environment-key" || cfg.SiliconFlow.Model != "environment-model" || cfg.SiliconFlow.BaseURL != "http://127.0.0.1/chat" {
		t.Fatal("environment did not override file configuration")
	}
	if cfg.SiliconFlow.Temperature != 0.2 || !reflect.DeepEqual(cfg.SiliconFlow.FallbackModels, []string{"model-a", "model-b"}) {
		t.Fatal("file options or fallback models were not preserved correctly")
	}
}

func TestConfigFileStillWorks(t *testing.T) {
	clearConfigEnv(t)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"listen_port":8067,"siliconflow":{"api_key":"file-key","model":"file-model"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil || cfg.ListenPort != 8067 || cfg.SiliconFlow.APIKey != "file-key" || cfg.SiliconFlow.Model != "file-model" {
		t.Fatalf("existing configuration failed: %v", err)
	}
}

func TestConfigWithoutFile(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SILICONFLOW_API_KEY", "environment-key")
	cfg, err := loadConfig(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenPort != 8066 || cfg.SiliconFlow.Model == "" || cfg.SiliconFlow.BaseURL == "" {
		t.Fatal("default deployment configuration is incomplete")
	}
	if len(cfg.SiliconFlow.FallbackModels) != 0 {
		t.Fatal("an explicitly empty fallback variable should disable fallback models")
	}
}

func TestConfigRejectsInvalidDeploymentSettings(t *testing.T) {
	for _, port := range []string{"not-a-port", "0", "65536"} {
		t.Run(port, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("LISTEN_PORT", port)
			t.Setenv("SILICONFLOW_API_KEY", "environment-key")
			if _, err := loadConfig(filepath.Join(t.TempDir(), "missing.json")); err == nil {
				t.Fatal("invalid port was accepted")
			}
		})
	}
	t.Run("missing key", func(t *testing.T) {
		clearConfigEnv(t)
		if _, err := loadConfig(filepath.Join(t.TempDir(), "missing.json")); err == nil {
			t.Fatal("missing API key was accepted")
		}
	})
}
