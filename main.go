package main

import (
	"flag"
	"fmt"
	"os"

	"godump/backup"
	"godump/config"
	"godump/logger"
	"godump/web"
)

var Version = "dev"

func main() {
	configPath := flag.String("config", "/etc/godump/config.yaml", "Path to configuration file")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()

	if *showVersion {
		fmt.Println(Version)
		return
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("Config file %s does not exist. Generating a default config...\n", *configPath)
			if genErr := config.GenerateDefaultConfig(*configPath); genErr != nil {
				fmt.Fprintf(os.Stderr, "Error generating default configuration: %v\n", genErr)
				os.Exit(1)
			}
			cfg, err = config.LoadConfig(*configPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading generated configuration: %v\n", err)
				os.Exit(1)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Error loading configuration from %s: %v\n", *configPath, err)
			os.Exit(1)
		}
	}

	if err := logger.Init(cfg.Logging.File); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing logger: %v\n", err)
		os.Exit(1)
	}

	logger.Info("", "Starting GoDump...")

	manager := backup.NewManager(cfg)
	
	logger.Info("", "Running initial database discovery...")
	manager.DiscoverInitial()

	server := web.NewServer(cfg, manager)
	if err := server.Start(); err != nil {
		logger.Error("", "Server stopped: %v", err)
		os.Exit(1)
	}
}
