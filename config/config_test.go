package config

import (
	"testing"
)

func TestConfig(t *testing.T) {
	cfg := Config{}
	if cfg != (Config{}) {
		t.Error("Config initialization failed")
	}
}
