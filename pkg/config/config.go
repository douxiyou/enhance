package config

import (
	"douxiyou.com/enhance/pkg/storage"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var globalConfig *Config

type Config struct {
	DataDir        string `mapstructure:"data_dir" default:"."`
	logger         *zap.Logger
	Instance       Instance `mapstructure:"instance"`
	Debug          bool
	ListenOnlyMode bool       `mapstructure:"listen_only_mode" default:"false"`
	Mqtt           MQTTConfig `mapstructure:"mqtt"`
	Badger         BadgerConfig `mapstructure:"badger"`
	Dhcp           DHCPConfig `mapstructure:"dhcp"`
}

type BadgerConfig struct {
	Prefix string `mapstructure:"prefix" default:"/enhance"`
}

type Instance struct {
	Identifier string `mapstructure:"identifier" default:""`
	IP         string `mapstructure:"ip" default:""`
	Listen     string `mapstructure:"listen" default:""`
}

type MQTTConfig struct {
	AutoStart bool   `mapstructure:"auto_start" default:"true"`
	Address   string `mapstructure:"address" default:""`
	HTTP      string `mapstructure:"http" default:""`
}

type DHCPConfig struct {
	Interface string      `mapstructure:"interface" default:"en0"`
	Listen    string      `mapstructure:"listen" default:""`
	Scope     ScopeConfig `mapstructure:"scope" default:""`
}

type ScopeConfig struct {
	Name         string            `mapstructure:"name" default:""`
	Gateway      string            `mapstructure:"gateway" default:""`
	Mask         string            `mapstructure:"mask" default:""`
	SubnetCIDR   string            `mapstructure:"subnet_cidr" default:""`
	DNSServers   []string          `mapstructure:"dns_servers" default:""`
	TTL          int64             `mapstructure:"ttl" default:""`
	RangeStart   string            `mapstructure:"range_start" default:""`
	RangeEnd     string            `mapstructure:"range_end" default:""`
	ShouldPing   bool              `mapstructure:"should_ping" default:""`
	Reservations []DHCPReservation `mapstructure:"reservations" default:""`
}

type DHCPReservation struct {
	Mac      string `mapstructure:"mac" default:""`
	IP       string `mapstructure:"ip" default:""`
	Hostname string `mapstructure:"hostname" default:""`
}

func GetGlobalConfig() *Config {
	return globalConfig
}

func NewConfig(configPath string) error {
	viper.SetConfigName("config")
	viper.AddConfigPath(configPath)
	viper.SetConfigType("json")

	if err := viper.ReadInConfig(); err != nil {
		return err
	}

	globalConfig = &Config{}
	if err := viper.Unmarshal(&globalConfig); err != nil {
		return err
	}

	globalConfig.logger = globalConfig.InitLog()
	if err := globalConfig.Validate(); err != nil {
		globalConfig.logger.Debug("验证配置失败，使用默认配置", zap.Error(err))
	}
	return nil
}

func GenerateConfigFile(path string) error {
	config := &Config{}
	if err := viper.Unmarshal(config); err != nil {
		return err
	}
	if err := viper.WriteConfigAs(path); err != nil {
		return err
	}
	return nil
}

func (c *Config) StorageClient() *storage.Client {
	return storage.NewClient(
		c.Badger.Prefix,
		c.DataDir,
		c.logger,
		c.Debug,
	)
}

func (c *Config) Validate() error {
	return nil
}
