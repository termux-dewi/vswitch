package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"

	"vswitch/pkg/emulation"
)

type Config struct {
	WebUI      WebUIConfig     `json:"webui"`
	Network    NetworkConfig   `json:"network"`
	Transports TransportConfig `json:"transports"`
	Storage    StorageConfig   `json:"storage"`
	Emulation  EmulationConfig `json:"emulation"`
}
type WebUIConfig struct {
	Enabled    bool   `json:"enabled"`
	ListenAddr string `json:"listen_addr"`
}
type NetworkConfig struct {
	GatewayIP  string `json:"gateway_ip"`
	GatewayMAC string `json:"gateway_mac"`
	SubnetMask string `json:"subnet_mask"`
}
type TransportConfig struct {
	UDSFile     string `json:"uds_file"`
	UDSAbstract string `json:"uds_abstract"`
	UDP         string `json:"udp"`
	GRPC        string `json:"grpc"`
	TLSCert     string `json:"tls_cert,omitempty"`
	TLSKey      string `json:"tls_key,omitempty"`
}
type StorageConfig struct {
	LeasesFile string `json:"leases_file"`
}
type EmulationConfig struct {
	Profiles []EmulationEntry `json:"profiles"`
}
type EmulationEntry struct {
	Key     string             `json:"key"`
	Preset  string             `json:"preset"`
	Profile *emulation.Profile `json:"profile,omitempty"`
}

func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if err := Validate(&c); err != nil {
		return nil, err
	}
	return &c, nil
}
func SaveConfig(path string, cfg *Config) error {
	if err := Validate(cfg); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}
func Validate(c *Config) error {
	if net.ParseIP(c.Network.GatewayIP) == nil {
		return fmt.Errorf("invalid gateway_ip")
	}
	if net.ParseIP(c.Network.SubnetMask) == nil {
		return fmt.Errorf("invalid subnet_mask")
	}
	if len(c.Network.GatewayMAC) == 0 {
		return fmt.Errorf("gateway_mac is required")
	}
	if c.Transports.UDSFile == "" || c.Transports.UDP == "" || c.Transports.GRPC == "" {
		return fmt.Errorf("transport addresses are required")
	}
	if c.Storage.LeasesFile == "" {
		return fmt.Errorf("leases_file is required")
	}
	return nil
}
