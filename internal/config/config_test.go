package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigLoader_LoadDefaults(t *testing.T) {
	loader := NewConfigLoader()
	var cfg ServerCmdConfig

	// Create a temporary directory for config
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")

	// Create empty config file
	err := os.WriteFile(configPath, []byte(""), 0644)
	require.NoError(t, err)

	// Create a test command
	cmd := &cobra.Command{
		Use: "test",
	}

	// Register flags (this adds the config flag)
	loader.RegisterFlags(cmd.Flags(), reflect.TypeFor[ServerCmdConfig]())

	// Set the config flag value
	require.NoError(t, cmd.Flags().Set("config", configPath))

	// Load config
	err = loader.Load(cmd, &cfg)
	require.NoError(t, err)

	// Test default values from struct tags
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 10*time.Second, cfg.Server.GracefulShutdown)
	assert.Equal(t, time.Hour, cfg.Server.ReadTimeout)
	assert.Equal(t, time.Hour, cfg.Server.WriteTimeout)
	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, 10485760, cfg.Cache.MaxSize)
	assert.Equal(t, true, cfg.DB.Pool.Enable)
	assert.Equal(t, 25, cfg.DB.Pool.MaxOpenConnections)
	assert.Equal(t, 25, cfg.DB.Pool.MaxIdleConnections)
	assert.Equal(t, 10*time.Minute, cfg.DB.Pool.MaxLifetime)
	assert.Equal(t, true, cfg.DB.PrepareStmt)
	assert.Equal(t, "error", cfg.DB.LogLevel)
	assert.Equal(t, true, cfg.CronJobs.Enable)
	assert.Equal(t, "cron-locker", cfg.CronJobs.LockerInstance)
	assert.Equal(t, time.Hour, cfg.CronJobs.CleanFilesInterval)
	assert.Equal(t, 12*time.Hour, cfg.CronJobs.CleanUploadsInterval)
	assert.Equal(t, 2*time.Hour, cfg.CronJobs.FolderSizeInterval)
	assert.Equal(t, true, cfg.TG.RateLimit)
	assert.Equal(t, 5, cfg.TG.RateBurst)
	assert.Equal(t, 100, cfg.TG.Rate)
	assert.Equal(t, 5*time.Minute, cfg.TG.ReconnectTimeout)
	assert.Equal(t, 8, cfg.TG.PoolSize)
	assert.Equal(t, "teldrive", cfg.TG.SessionInstance)
	assert.Equal(t, true, cfg.TG.AutoChannelCreate)
	assert.Equal(t, int64(500000), cfg.TG.ChannelLimit)
	assert.Equal(t, 1, cfg.TG.Stream.Concurrency)
	assert.Equal(t, 8, cfg.TG.Stream.Buffers)
	assert.Equal(t, 30*time.Second, cfg.TG.Stream.ChunkTimeout)
	assert.Equal(t, 8, cfg.TG.Uploads.Threads)
	assert.Equal(t, 10, cfg.TG.Uploads.MaxRetries)
	assert.Equal(t, 7*24*time.Hour, cfg.TG.Uploads.Retention)
	assert.Equal(t, 30*24*time.Hour, cfg.JWT.SessionTime)
}

func TestConfigLoader_LoadFromConfigFile(t *testing.T) {
	loader := NewConfigLoader()
	var cfg ServerCmdConfig

	// Create a temporary directory for config
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")

	// Create config file with custom values
	configContent := `
[server]
port = 9000
graceful-shutdown = "20s"

[log]
level = "debug"

[cache]
max-size = 20971520

[tg]
rate = 200
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Create a test command
	cmd := &cobra.Command{
		Use: "test",
	}

	// Register flags (this adds the config flag)
	loader.RegisterFlags(cmd.Flags(), reflect.TypeFor[ServerCmdConfig]())

	// Set the config flag value
	require.NoError(t, cmd.Flags().Set("config", configPath))

	// Load config
	err = loader.Load(cmd, &cfg)
	require.NoError(t, err)

	// Test that config file values override defaults
	assert.Equal(t, 9000, cfg.Server.Port)
	assert.Equal(t, 20*time.Second, cfg.Server.GracefulShutdown)
	assert.Equal(t, "debug", cfg.Log.Level)
	assert.Equal(t, 20971520, cfg.Cache.MaxSize)
	assert.Equal(t, 200, cfg.TG.Rate)

	// Test that other values still use defaults
	assert.Equal(t, time.Hour, cfg.Server.ReadTimeout)
	assert.Equal(t, time.Hour, cfg.Server.WriteTimeout)
}

func TestConfigLoader_CommandLineFlags(t *testing.T) {
	loader := NewConfigLoader()
	var cfg ServerCmdConfig

	// Create a temporary directory for config
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")

	// Create empty config file
	err := os.WriteFile(configPath, []byte(""), 0644)
	require.NoError(t, err)

	// Create a test command
	cmd := &cobra.Command{
		Use: "test",
	}

	// Register flags (this adds the config flag)
	loader.RegisterFlags(cmd.Flags(), reflect.TypeFor[ServerCmdConfig]())

	// Set the config flag value
	cmd.Flags().Set("config", configPath)

	// Set command line flags
	require.NoError(t, cmd.Flags().Set("server-port", "9999"))
	require.NoError(t, cmd.Flags().Set("log-level", "warn"))
	require.NoError(t, cmd.Flags().Set("cache-max-size", "31457280"))

	// Load config
	err = loader.Load(cmd, &cfg)
	require.NoError(t, err)

	// Test that command line flags override defaults
	assert.Equal(t, 9999, cfg.Server.Port)
	assert.Equal(t, "warn", cfg.Log.Level)
	assert.Equal(t, 31457280, cfg.Cache.MaxSize)
}

func TestConfigLoader_RequiredFields(t *testing.T) {
	loader := NewConfigLoader()
	var cfg ServerCmdConfig

	// Create a temporary directory for config
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")

	// Create config file without required fields
	configContent := `
[server]
port = 8080
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Create a test command
	cmd := &cobra.Command{
		Use: "test",
	}

	// Register flags (this adds the config flag)
	loader.RegisterFlags(cmd.Flags(), reflect.TypeFor[ServerCmdConfig]())

	// Set the config flag value
	require.NoError(t, cmd.Flags().Set("config", configPath))

	// Load config
	err = loader.Load(cmd, &cfg)
	require.NoError(t, err)

	// Validate should fail due to missing required fields
	err = loader.Validate(&cfg)
	assert.Error(t, err)
	// assert.Contains(t, err.Error(), "required configuration values not set")
	// Validator error messages are detailed
	assert.Contains(t, err.Error(), "failed on the 'required' tag")
}

func TestConfigLoader_LoadFromYAMLConfigFile(t *testing.T) {
	loader := NewConfigLoader()
	var cfg ServerCmdConfig

	// Create a temporary directory for config
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	// Create YAML config file with custom values
	configContent := `
server:
  port: 9000
  graceful-shutdown: "20s"
log:
  level: "debug"
cache:
  max-size: 20971520
tg:
  rate: 200
  rate-limit: false
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Create a test command
	cmd := &cobra.Command{
		Use: "test",
	}

	// Register flags (this adds the config flag)
	loader.RegisterFlags(cmd.Flags(), reflect.TypeFor[ServerCmdConfig]())

	// Set the config flag value
	require.NoError(t, cmd.Flags().Set("config", configPath))

	// Load config
	err = loader.Load(cmd, &cfg)
	require.NoError(t, err)

	// Test that YAML config file values override defaults
	assert.Equal(t, 9000, cfg.Server.Port)
	assert.Equal(t, 20*time.Second, cfg.Server.GracefulShutdown)
	assert.Equal(t, "debug", cfg.Log.Level)
	assert.Equal(t, 20971520, cfg.Cache.MaxSize)
	assert.Equal(t, 200, cfg.TG.Rate)
	assert.Equal(t, false, cfg.TG.RateLimit)

	// Test that other values still use defaults
	assert.Equal(t, time.Hour, cfg.Server.ReadTimeout)
	assert.Equal(t, time.Hour, cfg.Server.WriteTimeout)
	assert.Equal(t, true, cfg.DB.Pool.Enable)
	assert.Equal(t, 25, cfg.DB.Pool.MaxOpenConnections)
}

func TestConfigLoader_FlagDefaults(t *testing.T) {
	loader := NewConfigLoader()

	// Create a test command
	cmd := &cobra.Command{
		Use: "test",
	}

	// Register flags
	loader.RegisterFlags(cmd.Flags(), reflect.TypeFor[ServerCmdConfig]())

	// Check that flags have correct default values
	portFlag := cmd.Flags().Lookup("server-port")
	require.NotNil(t, portFlag)
	assert.Equal(t, "8080", portFlag.DefValue)

	logLevelFlag := cmd.Flags().Lookup("log-level")
	require.NotNil(t, logLevelFlag)
	assert.Equal(t, "info", logLevelFlag.DefValue)

	cacheSizeFlag := cmd.Flags().Lookup("cache-max-size")
	require.NotNil(t, cacheSizeFlag)
	assert.Equal(t, "10485760", cacheSizeFlag.DefValue)

	rateLimitFlag := cmd.Flags().Lookup("tg-rate-limit")
	require.NotNil(t, rateLimitFlag)
	assert.Equal(t, "true", rateLimitFlag.DefValue)

	rateFlag := cmd.Flags().Lookup("tg-rate")
	require.NotNil(t, rateFlag)
	assert.Equal(t, "100", rateFlag.DefValue)
}

func TestConfigLoader_LoadFromEnv(t *testing.T) {
	loader := NewConfigLoader()
	var cfg ServerCmdConfig
	cmd := &cobra.Command{Use: "test"}
	loader.RegisterFlags(cmd.Flags(), reflect.TypeFor[ServerCmdConfig]())

	// Set env vars
	require.NoError(t, os.Setenv("TELDRIVE_SERVER_PORT", "7070"))
	require.NoError(t, os.Setenv("TELDRIVE_LOG_LEVEL", "debug"))
	// Nested key
	require.NoError(t, os.Setenv("TELDRIVE_TG_UPLOADS_THREADS", "16"))

	defer func() { _ = os.Unsetenv("TELDRIVE_SERVER_PORT") }()
	defer func() { _ = os.Unsetenv("TELDRIVE_LOG_LEVEL") }()
	defer func() { _ = os.Unsetenv("TELDRIVE_TG_UPLOADS_THREADS") }()

	err := loader.Load(cmd, &cfg)
	require.NoError(t, err)

	assert.Equal(t, 7070, cfg.Server.Port)
	assert.Equal(t, "debug", cfg.Log.Level)
	assert.Equal(t, 16, cfg.TG.Uploads.Threads)
}

func TestConfigLoader_Priority(t *testing.T) {
	// Priority: Flag > Env > File > Defaults

	loader := NewConfigLoader()
	var cfg ServerCmdConfig

	// Create config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")
	err := os.WriteFile(configPath, []byte("[server]\nport = 9000"), 0644)
	require.NoError(t, err)

	cmd := &cobra.Command{Use: "test"}
	loader.RegisterFlags(cmd.Flags(), reflect.TypeFor[ServerCmdConfig]())
	cmd.Flags().Set("config", configPath)

	// 1. File overrides Default (8080)
	err = loader.Load(cmd, &cfg)
	require.NoError(t, err)
	assert.Equal(t, 9000, cfg.Server.Port)

	// 2. Env overrides File
	require.NoError(t, os.Setenv("TELDRIVE_SERVER_PORT", "7000"))
	defer func() { _ = os.Unsetenv("TELDRIVE_SERVER_PORT") }()

	err = loader.Load(cmd, &cfg)
	require.NoError(t, err)
	assert.Equal(t, 7000, cfg.Server.Port)

	// 3. Flag overrides Env
	require.NoError(t, cmd.Flags().Set("server-port", "6000"))

	err = loader.Load(cmd, &cfg)
	require.NoError(t, err)
	assert.Equal(t, 6000, cfg.Server.Port)
}
