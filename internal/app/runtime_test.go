package app

import (
	"os"
	"testing"
)

func TestEnvInt(t *testing.T) {
	os.Setenv("TEST_ENV_INT", "123")
	defer os.Unsetenv("TEST_ENV_INT")
	if val := EnvInt("TEST_ENV_INT", 456); val != 123 {
		t.Errorf("expected 123, got %d", val)
	}

	if val := EnvInt("TEST_ENV_NONEXIST", 456); val != 456 {
		t.Errorf("expected 456, got %d", val)
	}

	os.Setenv("TEST_ENV_INT_INVALID", "abc")
	defer os.Unsetenv("TEST_ENV_INT_INVALID")
	if val := EnvInt("TEST_ENV_INT_INVALID", 456); val != 456 {
		t.Errorf("expected 456, got %d", val)
	}
}

func TestGetenv(t *testing.T) {
	os.Setenv("TEST_GETENV", "val")
	defer os.Unsetenv("TEST_GETENV")
	if val := getenv("TEST_GETENV", "fallback"); val != "val" {
		t.Errorf("expected val, got %s", val)
	}

	if val := getenv("TEST_GETENV_NONEXIST", "fallback"); val != "fallback" {
		t.Errorf("expected fallback, got %s", val)
	}
}

func TestServiceAddr(t *testing.T) {
	// Test defaultPort <= 0 path
	// (Note that config.GlobalConfig might not be fully initialized in pure unit test, 
	// but it defaults to config.Server.Port. Let's make sure it doesn't crash)
	
	// Test with PORT env
	os.Setenv("PORT", "9999")
	if addr := serviceAddr(8080); addr != ":9999" {
		t.Errorf("expected :9999, got %s", addr)
	}
	os.Unsetenv("PORT")

	// Test with GOSHOP_SERVICE_PORT env
	os.Setenv("GOSHOP_SERVICE_PORT", "8888")
	if addr := serviceAddr(8080); addr != ":8888" {
		t.Errorf("expected :8888, got %s", addr)
	}
	os.Unsetenv("GOSHOP_SERVICE_PORT")

	// Test with defaultPort
	if addr := serviceAddr(7777); addr != ":7777" {
		t.Errorf("expected :7777, got %s", addr)
	}
}
