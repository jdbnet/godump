package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type AuthConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type Config struct {
	Server    ServerConfig     `yaml:"server"`
	Auth      AuthConfig       `yaml:"auth"`
	Logging   LoggingConfig    `yaml:"logging"`
	Instances []InstanceConfig `yaml:"instances"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type LoggingConfig struct {
	File string `yaml:"file"`
}

type InstanceConfig struct {
	Name          string `yaml:"name"`
	Host          string `yaml:"host"`
	Port          int    `yaml:"port"`
	User          string `yaml:"user"`
	Password      string `yaml:"password"`
	BackupDir     string `yaml:"backup_dir"`
	RetentionDays int    `yaml:"retention_days"`
	Schedule      string `yaml:"schedule"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func GenerateDefaultConfig(path string) error {
	defaultCfg := Config{
		Server: ServerConfig{
			Port: 8080,
		},
		Auth: AuthConfig{
			Enabled:  false,
			Username: "admin",
			Password: "password",
		},
		Logging: LoggingConfig{
			File: "",
		},
		Instances: []InstanceConfig{
			{
				Name:          "primary",
				Host:          "127.0.0.1",
				Port:          3306,
				User:          "backup",
				Password:      "secret",
				BackupDir:     "/backups/primary",
				RetentionDays: 14,
				Schedule:      "0 2 * * *",
			},
		},
	}

	data, err := yaml.Marshal(&defaultCfg)
	if err != nil {
		return err
	}

	// Make sure the directory exists
	dir := os.Args[0]
	for i := len(path) - 1; i >= 0 && !os.IsPathSeparator(path[i]); i-- {
		dir = path[:i]
	}
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return os.WriteFile(path, data, 0644)
}
