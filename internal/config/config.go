package config

import (
	"fmt"
	"os"
	"strings"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/kelseyhightower/envconfig"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Logging       LoggingConfig  `yaml:"logging"`
	Debug         DebugConfig    `yaml:"debug"`
	Submit        SubmitConfig   `yaml:"submit"`
	Topology      TopologyConfig `yaml:"topology"`
	Storage       StorageConfig  `yaml:"storage"`
	Indexer       IndexerConfig  `yaml:"indexer"`
	Wallet        WalletConfig   `yaml:"wallet"`
	Network       string         `yaml:"network" envconfig:"NETWORK"`
	Profiles      []string       `yaml:"profiles" envconfig:"PROFILES"`
	ListenAddress string         `yaml:"listenAddress" envconfig:"LISTEN_ADDRESS"`
	ListenPort    uint           `yaml:"port" envconfig:"PORT"`
	NetworkMagic  uint32
}

type LoggingConfig struct {
	Level string `yaml:"level" envconfig:"LOGGING_LEVEL"`
}

type DebugConfig struct {
	ListenAddress string `yaml:"address" envconfig:"DEBUG_ADDRESS"`
	ListenPort    uint   `yaml:"port" envconfig:"DEBUG_PORT"`
}

type TopologyConfig struct {
	ConfigFile string               `yaml:"configFile" envconfig:"CARDANO_TOPOLOGY"`
	Hosts      []TopologyConfigHost `yaml:"hosts"`
}

type TopologyConfigHost struct {
	Address string `yaml:"address"`
	Port    uint   `yaml:"port"`
}

type IndexerConfig struct {
	Address    string `yaml:"address"       envconfig:"INDEXER_TCP_ADDRESS"`
	SocketPath string `yaml:"socketPath"    envconfig:"INDEXER_SOCKET_PATH"`
}

type SubmitConfig struct {
	Address    string `yaml:"address"      envconfig:"SUBMIT_TCP_ADDRESS"`
	SocketPath string `yaml:"socketPath"   envconfig:"SUBMIT_SOCKET_PATH"`
	Url        string `yaml:"url"          envconfig:"SUBMIT_URL"`
}

type StorageConfig struct {
	Directory string `yaml:"dir" envconfig:"STORAGE_DIR"`
}

type WalletConfig struct {
	Mnemonic string `yaml:"mnemonic" envconfig:"MNEMONIC"`
}

// Singleton config instance with default values
var globalConfig = &Config{
	Network:    "mainnet",
	Profiles:   []string{"spectrum", "teddyswap"},
	ListenPort: 3000,
	Logging: LoggingConfig{
		Level: "info",
	},
	Debug: DebugConfig{
		ListenAddress: "localhost",
		ListenPort:    0,
	},
	Storage: StorageConfig{
		// TODO: pick a better location
		Directory: "./.shai",
	},
}

func Load(configFile string) (*Config, error) {
	// Load config file as YAML if provided
	if configFile != "" {
		buf, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("error reading config file: %s", err)
		}
		err = yaml.Unmarshal(buf, globalConfig)
		if err != nil {
			return nil, fmt.Errorf("error parsing config file: %s", err)
		}
	}
	// Load config values from environment variables
	// We use "dummy" as the app name here to (mostly) prevent picking up env
	// vars that we hadn't explicitly specified in annotations above
	err := envconfig.Process("dummy", globalConfig)
	if err != nil {
		return nil, fmt.Errorf("error processing environment: %s", err)
	}
	// Load topology config file, if specified
	if globalConfig.Topology.ConfigFile != "" {
		if err := globalConfig.loadTopologyConfig(); err != nil {
			return nil, err
		}
	}
	// Populate network magic from network name
	network := ouroboros.NetworkByName(globalConfig.Network)
	if network == ouroboros.NetworkInvalid {
		return nil, fmt.Errorf("unknown network name: %s", globalConfig.Network)
	}
	globalConfig.NetworkMagic = network.NetworkMagic
	// Check profiles
	availableProfiles := GetAvailableProfiles()
	for _, profile := range globalConfig.Profiles {
		foundProfile := false
		for _, availableProfile := range availableProfiles {
			if profile == availableProfile {
				foundProfile = true
				break
			}
		}
		if !foundProfile {
			return nil, fmt.Errorf("unknown profile: %s: available profiles: %s", profile, strings.Join(availableProfiles, ","))
		}
	}
	return globalConfig, nil
}

func (cfg *Config) loadTopologyConfig() error {
	topology, err := ouroboros.NewTopologyConfigFromFile(cfg.Topology.ConfigFile)
	if err != nil {
		return err
	}
	// Legacy topology config
	for _, host := range topology.Producers {
		cfg.Topology.Hosts = append(
			cfg.Topology.Hosts,
			TopologyConfigHost{
				Address: host.Address,
				Port:    uint(host.Port),
			},
		)
	}
	// P2P local roots
	for _, localRoot := range topology.LocalRoots {
		for _, host := range localRoot.AccessPoints {
			cfg.Topology.Hosts = append(
				cfg.Topology.Hosts,
				TopologyConfigHost{
					Address: host.Address,
					Port:    uint(host.Port),
				},
			)
		}
	}
	// P2P public roots
	for _, publicRoot := range topology.PublicRoots {
		for _, host := range publicRoot.AccessPoints {
			cfg.Topology.Hosts = append(
				cfg.Topology.Hosts,
				TopologyConfigHost{
					Address: host.Address,
					Port:    uint(host.Port),
				},
			)
		}
	}
	return nil
}

// Return global config instance
func GetConfig() *Config {
	return globalConfig
}
