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

type EmailConfig struct {
	Enabled  bool                      `yaml:"enabled"`
	Events   *NotificationEventsConfig `yaml:"events,omitempty"`
	Host     string                    `yaml:"host"`
	Port     int                       `yaml:"port"`
	Username string                    `yaml:"username"`
	Password string                    `yaml:"password"`
	From     string                    `yaml:"from"`
	To       string                    `yaml:"to"`
}

type WebhookConfig struct {
	Enabled  bool                      `yaml:"enabled"`
	Events   *NotificationEventsConfig `yaml:"events,omitempty"`
	URL      string                    `yaml:"url"`
	Headers  map[string]string         `yaml:"headers"`
}

type NotificationEventsConfig struct {
	OnSuccess bool `yaml:"on_success"`
	OnFailure bool `yaml:"on_failure"`
}

type NotificationsConfig struct {
	Events   NotificationEventsConfig `yaml:"events"`
	Email    EmailConfig              `yaml:"email"`
	Webhook  WebhookConfig            `yaml:"webhook"`
	Webhooks []WebhookConfig          `yaml:"webhooks"`
}

type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Auth          AuthConfig          `yaml:"auth"`
	Notifications NotificationsConfig `yaml:"notifications"`
	Logging       LoggingConfig       `yaml:"logging"`
	Instances     []InstanceConfig    `yaml:"instances"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type LoggingConfig struct {
	File string `yaml:"file"`
}

type InstanceConfig struct {
	Name          string   `yaml:"name"`
	Host          string   `yaml:"host"`
	Port          int      `yaml:"port"`
	User          string   `yaml:"user"`
	Password      string   `yaml:"password"`
	BackupDir     string   `yaml:"backup_dir"`
	RetentionDays int      `yaml:"retention_days"`
	Schedule      string   `yaml:"schedule"`
	Include       []string `yaml:"include,omitempty"`
	Exclude       []string `yaml:"exclude,omitempty"`
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
		Notifications: NotificationsConfig{
			Events: NotificationEventsConfig{
				OnSuccess: false,
				OnFailure: true,
			},
			Email: EmailConfig{
				Enabled: false,
				Host:    "smtp.example.com",
				Port:    587,
			},
			Webhook: WebhookConfig{
				Enabled: false,
				URL:     "https://hook.example.com",
			},
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
