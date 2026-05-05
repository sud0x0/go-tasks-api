package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration loaded from environment variables.
// Load this once at startup and pass it explicitly to components that need it.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Log      LogConfig
	Valkey   ValkeyConfig
	JWT      JWTConfig
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Port string
}

// DatabaseConfig holds database connection configuration.
type DatabaseConfig struct {
	Host            string
	Port            string
	User            string
	Password        string
	Name            string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// LogConfig holds logging configuration.
type LogConfig struct {
	Level string
}

// ValkeyConfig holds Valkey (Redis-compatible) configuration.
type ValkeyConfig struct {
	URL      string
	Password string
}

// JWTConfig holds JWT authentication configuration.
type JWTConfig struct {
	Issuer         string
	Audience       string
	PrivateKeyPath string
	PublicKeyPath  string
}

// Load reads all environment variables and returns a validated Config.
// Returns an error if required variables are missing.
func Load() (*Config, error) {
	dbCfg, err := LoadDatabase()
	if err != nil {
		return nil, err
	}

	return &Config{
		Server: ServerConfig{
			Port: getEnv("PORT", "8080"),
		},
		Database: *dbCfg,
		Log: LogConfig{
			Level: strings.ToLower(getEnv("LOG_LEVEL", "development")),
		},
		Valkey: ValkeyConfig{
			URL:      getEnv("VALKEY_URL", "localhost:6379"),
			Password: getEnv("VALKEY_PASSWORD", ""),
		},
		JWT: JWTConfig{
			Issuer:         getEnv("JWT_ISSUER", "go-tasks-api"),
			Audience:       getEnv("JWT_AUDIENCE", "go-tasks-api"),
			PrivateKeyPath: getEnv("JWT_PRIVATE_KEY_PATH", "./keys/private.pem"),
			PublicKeyPath:  getEnv("JWT_PUBLIC_KEY_PATH", "./keys/public.pem"),
		},
	}, nil
}

// LoadDatabase reads only the DB_* variables and returns a validated
// DatabaseConfig. The migrator binary uses this directly so it shares the
// same env var names as the API server and operators only need to set one
// set of variables.
func LoadDatabase() (*DatabaseConfig, error) {
	cfg := &DatabaseConfig{
		Host:            getEnv("DB_HOST", ""),
		Port:            getEnv("DB_PORT", ""),
		User:            getEnv("DB_USER", ""),
		Password:        getEnv("DB_PASSWORD", ""),
		Name:            getEnv("DB_NAME", ""),
		SSLMode:         getEnv("DB_SSLMODE", "require"),
		MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 100),
		MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 50),
		ConnMaxLifetime: time.Duration(getEnvInt("DB_CONN_MAX_LIFETIME_MINS", 5)) * time.Minute,
		ConnMaxIdleTime: time.Duration(getEnvInt("DB_CONN_MAX_IDLE_TIME_MINS", 10)) * time.Minute,
	}

	if cfg.Host == "" || cfg.Port == "" || cfg.User == "" || cfg.Password == "" || cfg.Name == "" {
		return nil, fmt.Errorf("missing required database environment variables (DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME)")
	}

	return cfg, nil
}

// ConnectionString returns the PostgreSQL connection string.
func (c *DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode,
	)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return intValue
}
