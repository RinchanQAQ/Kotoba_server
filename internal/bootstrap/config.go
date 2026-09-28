package bootstrap

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/spf13/viper"
)

// Config 汇总应用的全部配置项。
type Config struct {
	App   AppConfig   `mapstructure:"app"`
	Log   LogConfig   `mapstructure:"log"`
	MySQL MySQLConfig `mapstructure:"mysql"`
	Redis RedisConfig `mapstructure:"redis"`
}

// AppConfig 应用自身的行为配置。
type AppConfig struct {
	Name            string        `mapstructure:"name"`
	Env             string        `mapstructure:"env"`
	Addr            string        `mapstructure:"addr"`
	ReadTimeout     time.Duration `mapstructure:"-"`
	WriteTimeout    time.Duration `mapstructure:"-"`
	ShutdownTimeout time.Duration `mapstructure:"-"`
	CORS            CORSConfig    `mapstructure:"cors"`

	// 以下字段以秒为单位从配置文件读取，再换算成 time.Duration。
	ReadTimeoutSec     int `mapstructure:"read_timeout"`
	WriteTimeoutSec    int `mapstructure:"write_timeout"`
	ShutdownTimeoutSec int `mapstructure:"shutdown_timeout"`
}

// CORSConfig 跨域配置。
type CORSConfig struct {
	AllowOrigins []string `mapstructure:"allow_origins"`
}

// LogConfig 日志配置。
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// MySQLConfig 数据库配置。
type MySQLConfig struct {
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	Username        string `mapstructure:"username"`
	Password        string `mapstructure:"password"`
	Database        string `mapstructure:"database"`
	Charset         string `mapstructure:"charset"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"` // 秒
	LogLevel        string `mapstructure:"log_level"`
}

// RedisConfig 缓存配置。
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

// DSN 拼装 gorm mysql 驱动所需的连接串。
func (c MySQLConfig) DSN() string {
	charset := c.Charset
	if charset == "" {
		charset = "utf8mb4"
	}

	opts := driver.NewConfig()
	opts.User = c.Username
	opts.Passwd = c.Password
	opts.Net = "tcp"
	opts.Addr = net.JoinHostPort(c.Host, fmt.Sprintf("%d", c.Port))
	opts.DBName = c.Database
	opts.ParseTime = true
	opts.Loc = time.Local
	opts.Params = map[string]string{"charset": charset}
	opts.Timeout = 5 * time.Second
	opts.ReadTimeout = 10 * time.Second
	opts.WriteTimeout = 10 * time.Second

	return opts.FormatDSN()
}

// Addr 返回 Redis 的 host:port。
func (c RedisConfig) Addr() string {
	return net.JoinHostPort(c.Host, fmt.Sprintf("%d", c.Port))
}

// IsProduction 判断是否运行在生产模式。
func (a AppConfig) IsProduction() bool {
	return strings.EqualFold(a.Env, "production")
}

// LoadConfig 读取配置文件并完成默认值填充与校验。
//
// 除主配置文件外，若同目录下存在 config.local.yaml，会自动合并覆盖，
// 便于把本地密码等敏感项排除在版本控制之外。
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
	}

	// 合并本地覆盖文件（可选）。
	localPath := filepath.Join(filepath.Dir(path), LocalConfigName)
	if _, err := os.Stat(localPath); err == nil {
		v.SetConfigFile(localPath)
		if err := v.MergeInConfig(); err != nil {
			return nil, fmt.Errorf("合并本地配置 %s 失败: %w", localPath, err)
		}
	}

	// 环境变量覆盖，例如 KOTOBA_MYSQL_PASSWORD。
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// 秒 -> time.Duration
	cfg.App.ReadTimeout = time.Duration(cfg.App.ReadTimeoutSec) * time.Second
	cfg.App.WriteTimeout = time.Duration(cfg.App.WriteTimeoutSec) * time.Second
	cfg.App.ShutdownTimeout = time.Duration(cfg.App.ShutdownTimeoutSec) * time.Second

	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.App.Name == "" {
		c.App.Name = "kotoba"
	}
	if c.App.Addr == "" {
		c.App.Addr = ":8080"
	}
	if c.App.ReadTimeoutSec <= 0 {
		c.App.ReadTimeoutSec = 15
	}
	if c.App.WriteTimeoutSec <= 0 {
		c.App.WriteTimeoutSec = 15
	}
	if c.App.ShutdownTimeoutSec <= 0 {
		c.App.ShutdownTimeoutSec = 10
	}

	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Format == "" {
		if c.App.IsProduction() {
			c.Log.Format = "json"
		} else {
			c.Log.Format = "console"
		}
	}

	if c.MySQL.Host == "" {
		c.MySQL.Host = "127.0.0.1"
	}
	if c.MySQL.Port == 0 {
		c.MySQL.Port = 3306
	}
	if c.MySQL.Charset == "" {
		c.MySQL.Charset = "utf8mb4"
	}
	if c.MySQL.MaxOpenConns <= 0 {
		c.MySQL.MaxOpenConns = 50
	}
	if c.MySQL.MaxIdleConns <= 0 {
		c.MySQL.MaxIdleConns = 10
	}
	if c.MySQL.ConnMaxLifetime <= 0 {
		c.MySQL.ConnMaxLifetime = 3600
	}
	if c.MySQL.LogLevel == "" {
		c.MySQL.LogLevel = "info"
	}

	if c.Redis.Host == "" {
		c.Redis.Host = "127.0.0.1"
	}
	if c.Redis.Port == 0 {
		c.Redis.Port = 6379
	}
	if c.Redis.PoolSize <= 0 {
		c.Redis.PoolSize = 50
	}
}

func (c *Config) validate() error {
	if c.MySQL.Database == "" {
		return fmt.Errorf("mysql.database 不能为空")
	}
	if c.MySQL.Username == "" {
		return fmt.Errorf("mysql.username 不能为空")
	}
	switch strings.ToLower(c.MySQL.LogLevel) {
	case "silent", "error", "warn", "info":
	default:
		return fmt.Errorf("mysql.log_level 取值非法: %s", c.MySQL.LogLevel)
	}
	return nil
}
