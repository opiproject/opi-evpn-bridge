package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestInitcfg_WithDefaultPath(t *testing.T) {
	// Ensure no custom config file is set
	GlobalConfig.CfgFile = ""

	// Set default config to avoid actual file dependencies
	viper.SetDefault("grpcport", 50051)
	viper.SetDefault("httpport", 8080)
	viper.SetDefault("dbaddress", "127.0.0.1:5432")

	Initcfg()

	// Validate defaults
	assert.Equal(t, uint16(50051), GetConfig().GRPCPort)
	assert.Equal(t, uint16(8080), GetConfig().HTTPPort)
	assert.Equal(t, "127.0.0.1:5432", GetConfig().DBAddress)
}

func TestSetAndGetConfig(t *testing.T) {
	cfg := Config{
		GRPCPort:  50051,
		HTTPPort:  8080,
		DBAddress: "127.0.0.1:5432",
	}

	err := SetConfig(cfg)
	assert.NoError(t, err)
	assert.Equal(t, &cfg, GetConfig())
}

func setupViperConfig(values map[string]interface{}) {
	viper.Reset()
	for key, value := range values {
		viper.Set(key, value)
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name: "Valid Config",
			config: map[string]interface{}{
				"grpcport":  50051,
				"httpport":  8080,
				"dbaddress": "127.0.0.1:5432",
			},
			wantErr: false,
		},
		{
			name: "Invalid GRPC Port",
			config: map[string]interface{}{
				"grpcport":  -1,
				"httpport":  8080,
				"dbaddress": "127.0.0.1:5432",
			},
			wantErr: true,
		},
		{
			name: "Invalid HTTP Port",
			config: map[string]interface{}{
				"grpcport":  50051,
				"httpport":  70000,
				"dbaddress": "127.0.0.1:5432",
			},
			wantErr: true,
		},
		{
			name: "Invalid DB Address Format",
			config: map[string]interface{}{
				"grpcport":  50051,
				"httpport":  8080,
				"dbaddress": "localhost",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupViperConfig(tt.config)
			err := ValidateConfig()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
