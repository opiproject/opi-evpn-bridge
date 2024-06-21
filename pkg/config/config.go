// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package config introduces the configuration from file or runtime param
package config

import (
	"fmt"
	"log"
	"net"
	"strconv"

	"github.com/spf13/viper"
)

// SubscriberConfig subscriber config structure
type SubscriberConfig struct {
	Name     string   `yaml:"name"`
	Priority int      `yaml:"priority"`
	Events   []string `yaml:"events"`
}

// P4FilesConfig p4 config structure
type P4FilesConfig struct {
	P4infoFile string `yaml:"p4infofile"`
	BinFile    string `yaml:"binfile"`
	ConfFile   string `yaml:"conffile"`
	SdePath    string `yaml:"sdepath"`
}

// RepresentorsConfig Representors config structure
type RepresentorsConfig struct {
	PortMux  string `yaml:"port-mux"`
	VrfMux   string `yaml:"vrf_mux"`
	GrpcAcc  string `yaml:"host"`
	GrpcHost string `yaml:"grpc_host"`
	Phy0Rep  string `yaml:"phy0_rep"`
	Phy1Rep  string `yaml:"phy1_rep"`
}

// P4Config p4 config structure
type P4Config struct {
	Enabled      bool                   `yaml:"enabled"`
	Driver       string                 `yaml:"driver"`
	Representors map[string]interface{} `yaml:"representors"`
	Config       P4FilesConfig          `yaml:"config"`
}

// loglevelConfig log level config structure
type loglevelConfig struct {
	DB      string `yaml:"db"`
	Grpc    string `yaml:"grpc"`
	Linux   string `yaml:"linux"`
	Netlink string `yaml:"netlink"`
	P4      string `yaml:"p4"`
}

// LinuxFrrConfig linux frr config structure
type LinuxFrrConfig struct {
	Enabled     bool   `yaml:"enabled"`
	DefaultVtep string `yaml:"defaultvtep"`
	PortMux     string `yaml:"portmux"`
	VrfMux      string `yaml:"vrfmux"`
	IPMtu       int    `yaml:"ipmtu"`
}

// NetlinkConfig netlink config structure
type NetlinkConfig struct {
	Enabled      bool `yaml:"enabled"`
	PollInterval int  `yaml:"pollinterval"`
	PhyPorts     []struct {
		Name string `yaml:"name"`
		Vsi  int    `yaml:"vsi"`
	} `yaml:"phyports"`
}

// Config global config structure
type Config struct {
	CfgFile     string
	GRPCPort    uint16             `yaml:"grpcport"`
	HTTPPort    uint16             `yaml:"httpport"`
	TLSFiles    string             `yaml:"tlsfiles"`
	Database    string             `yaml:"database"`
	DBAddress   string             `yaml:"dbaddress"`
	Buildenv    string             `yaml:"buildenv"`
	Tracer      bool               `yaml:"tracer"`
	Subscribers []SubscriberConfig `yaml:"subscribers"`
	LinuxFrr    LinuxFrrConfig     `yaml:"linuxfrr"`
	Netlink     NetlinkConfig      `yaml:"netlink"`
	P4          P4Config           `yaml:"p4"`
	LogLevel    loglevelConfig     `yaml:"loglevel"`
}

// GlobalConfig global config
var GlobalConfig Config

// SetConfig sets the global config
func SetConfig(cfg Config) error {
	GlobalConfig = cfg
	return nil
}

const (
	configFilePath = "./"
)

// Initcfg read the config from file
func Initcfg() {
	if GlobalConfig.CfgFile != "" {
		viper.SetConfigFile(GlobalConfig.CfgFile)
	} else {
		// Search config in the default location
		viper.AddConfigPath(configFilePath)
		viper.SetConfigType("yaml")
		viper.SetConfigName("config.yaml")
	}

	if err := LoadConfig(); err != nil {
		log.Panic(err)
	}
}

// LoadConfig loads the config from yaml file
func LoadConfig() error {
	if err := viper.ReadInConfig(); err != nil {
		// Handle errors
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Cfg file not found
			fmt.Println("Config file not found in default paths. Using defaults.")
		} else if _, ok := err.(viper.ConfigParseError); ok {
			// Cfg file parsing error
			log.Panicf("Error parsing config file: %v", err)
		} else {
			// Other errors
			log.Panicf("Error reading config file: %v", err)
		}
	}

	fmt.Println("Using config file:", viper.ConfigFileUsed())

	if err := viper.Unmarshal(&GlobalConfig); err != nil {
		log.Println(err)
		return err
	}

	if err := ValidateConfig(); err != nil {
		log.Panic(err)
	}

	log.Printf("config %+v", GlobalConfig)
	return nil
}

// GetConfig gets the global config
func GetConfig() *Config {
	return &GlobalConfig
}

// ValidateConfig validates the config parameters
func ValidateConfig() error {
	var err error

	grpcPort := viper.GetInt("grpcport")
	if grpcPort <= 0 || grpcPort > 65535 {
		err = fmt.Errorf("grpcPort must be a positive integer between 1 and 65535")
		return err
	}

	httpPort := viper.GetInt("httpport")
	if httpPort <= 0 || httpPort > 65535 {
		err = fmt.Errorf("httpPort must be a positive integer between 1 and 65535")
		return err
	}

	dbAddr := viper.GetString("dbaddress")
	_, port, err := net.SplitHostPort(dbAddr)
	if err != nil {
		err = fmt.Errorf("invalid DBAddress format. It should be in ip_address:port format")
		return err
	}

	dbPort, err := strconv.Atoi(port)
	if err != nil || dbPort <= 0 || dbPort > 65535 {
		err = fmt.Errorf("invalid db port. It must be a positive integer between 1 and 65535")
		return err
	}

	return nil
}
