package config

import (
	"douxiyou.com/enhance/pkg/storage"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var globalConfig *Config

type Config struct {
	logger         *zap.Logger
	Instance       Instance `json:"instance"`
	Debug          bool
	ListenOnlyMode bool `json:"listen_only_mode" default:"false"` // 是否仅监听模式
	// Mqtt           MQTTConfig `json:"mqtt"`                             // MQTT 配置
	Etcd EtcdConfig `json:"etcd"` // Etcd 配置
	Dhcp DHCPConfig `json:"dhcp"` // DHCP 配置
}
type EtcdConfig struct {
	JoinCluster string `json:"join_cluster"`                      // 加入集群的节点地址
	Prefix      string `json:"prefix" default:"/enhance"`         // Etcd 键前缀
	Endpoint    string `json:"endpoint" default:"localhost:2379"` // Etcd 客户端端口
	ClientPort  int32  `json:"client_port" default:"2381"`        // Etcd 客户端端口
	PeerPort    int32  `json:"peer_port" default:"2380"`          // Etcd 对等端口
}
type Instance struct {
	Identifier string `json:"identifier"` // 实例标识符
	IP         string `json:"ip"`         // 实例 IP 地址
	Interface  string `json:"interface"`  // 网络接口名称
	Listen     string `json:"listen"`     // 监听地址
}

// MQTTConfig MQTT 配置
// type MQTTConfig struct {
// 	AutoStart bool   `json:"auto_start"` // 是否自动启动 MQTT 服务器
// 	Address   string `json:"address"`    // MQTT 服务器监听地址
// 	HTTP      string `json:"http"`       // HTTP 统计信息监听地址
// }

// DHCPConfig DHCP 配置
type DHCPConfig struct {
	Interface string      `json:"interface" default:"en0"` // DHCP 服务器监听接口
	Listen    string      `json:"listen"`                  // DHCP 服务器监听地址
	Scope     ScopeConfig `json:"scope"`                   // DHCP 范围配置
}
type ScopeConfig struct {
	Name         string            `json:"name"`                                   // DHCP 范围名称
	Network      string            `json:"network"`                                // DHCP 网络范围
	Gateway      string            `json:"gateway"`                                // DHCP 网关地址
	SubnetCIDR   string            `json:"subnet_cidr" default:"255.255.255.0/24"` // DHCP 子网范围
	DNS          []string          `json:"dns_servers"`                            // DHCP DNS 服务器地址
	TTL          int64             `json:"ttl"`                                    // DHCP 租约时间
	RangeStart   string            `json:"range_start"`                            // DHCP IP 范围起始地址
	RangeEnd     string            `json:"range_end"`                              // DHCP IP 范围结束地址
	Reservations []DHCPReservation `json:"reservations"`                           // DHCP 保留 IP 配置
}
type DHCPReservation struct {
	Mac      string `json:"mac"`      // DHCP 保留 IP 配置 MAC 地址
	IP       string `json:"ip"`       // DHCP 保留 IP 配置 IP 地址
	Hostname string `json:"hostname"` // DHCP 保留 IP 配置主机名
}

func GetGlobalConfig() *Config {
	return globalConfig
}

// loadConfig 加载配置
func NewConfig(configPath string) error {
	// 初始化配置
	viper.SetConfigName("config")   // 配置文件名称（不带扩展名）
	viper.AddConfigPath(configPath) // 配置文件路径
	viper.SetConfigType("json")     // 配置文件类型

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		return err
	}

	// 解析配置到结构体
	if err := viper.Unmarshal(&globalConfig); err != nil {
		return err
	}

	// 先初始化 logger
	globalConfig.logger = globalConfig.InitLog()

	// 验证配置并设置默认值
	if err := globalConfig.Validate(); err != nil {
		globalConfig.Logger().Debug("验证配置失败，使用默认配置", zap.Error(err))
	}
	return nil
}

// GenerateConfigFile 生成配置文件
func GenerateConfigFile(path string) error {
	config := &Config{}
	// 解析配置到结构体
	if err := viper.Unmarshal(config); err != nil {
		return err
	}
	// 生成配置文件
	if err := viper.WriteConfigAs(path); err != nil {
		return err
	}
	return nil
}
func (c *Config) EtcdClient() *storage.Client {
	return storage.NewClient(
		"/enhance",
		c.logger,
		c.Debug,
	)
}

// Validate 验证配置是否有效 TODO 实现验证逻辑
func (c *Config) Validate() error {
	return nil
}
