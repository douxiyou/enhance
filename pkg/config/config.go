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
	ListenOnlyMode bool       `json:"listen_only_mode" default:"false"` // 是否仅监听模式
	Mqtt           MQTTConfig `json:"mqtt"`                             // MQTT 配置
	Etcd           EtcdConfig `json:"etcd"`                             // Etcd 配置
	Dhcp           DHCPConfig `json:"dhcp"`                             // DHCP 配置
}
type EtcdConfig struct {
	Prefix     string `json:"prefix" default:"/gravity"`
	Endpoint   string `json:"endpoint" default:"localhost:2379"`
	ClientPort int32  `json:"client_port" default:"2381"`
	PeerPort   int32  `json:"peer_port" default:"2380"`
}
type Instance struct {
	IP        string `json:"ip"`
	Interface string `json:"interface"`
	Listen    string `json:"listen"`
}

// MQTTConfig MQTT 配置
type MQTTConfig struct {
	AutoStart bool   `json:"auto_start"` // 是否自动启动 MQTT 服务器
	Address   string `json:"address"`    // MQTT 服务器监听地址
	HTTP      string `json:"http"`       // HTTP 统计信息监听地址
}

// DHCPConfig DHCP 配置
type DHCPConfig struct {
	ServerIP              string `json:"server_ip"`               // DHCP 服务器 IP 地址
	Gateway               string `json:"gateway"`                 // 网关 IP 地址（通常是路由器地址）
	ListenAddr            string `json:"listen_addr"`             // DHCP 服务器监听地址
	Interface             string `json:"interface"`               // 网络接口名称
	IPStart               string `json:"ip_start"`                // IP 池起始地址
	IPEnd                 string `json:"ip_end"`                  // IP 池结束地址
	Subnet                string `json:"subnet"`                  // 子网掩码
	LeaseTime             int    `json:"lease_time"`              // 租约时间（秒）
	LeaseNegotiateTimeout int    `json:"lease_negotiate_timeout"` // 租约协商超时时间（秒）
}

func Get() *Config {
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

	// 验证配置并设置默认值
	if err := globalConfig.Validate(); err != nil {
		globalConfig.Logger().Debug("验证配置失败，使用默认配置", zap.Error(err))
	}
	globalConfig.logger = globalConfig.InitLog()
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
