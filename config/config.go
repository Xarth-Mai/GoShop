package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   Server   `yaml:"server"`
	Database Database `yaml:"database"`
	Redis    Redis    `yaml:"redis"`
	JWT      JWT      `yaml:"jwt"`
}

type Server struct {
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"`
}

type Database struct {
	Master          string `yaml:"master"`
	Replica         string `yaml:"replica"`
	MaxIdleConns    int    `yaml:"max_idle_conns"`
	MaxOpenConns    int    `yaml:"max_open_conns"`
	ConnMaxLifetime int    `yaml:"conn_max_lifetime"`
}

type Redis struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	PoolSize int    `yaml:"pool_size"`
}

type JWT struct {
	Secret        string `yaml:"secret"`
	Expire        int    `yaml:"expire"`
	RefreshExpire int    `yaml:"refresh_expire"`
}

var GlobalConfig *Config

// InitConfig 加载并解析配置文件
func InitConfig(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return err
	}
	GlobalConfig = &cfg
	return nil
}
