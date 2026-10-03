package bootstrap

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/spf13/viper"
)

// LoadOptions 控制配置文件定位与环境选择。
// 两个字段留空时依次回落到环境变量（KOTOBA_CONFIG / KOTOBA_ENV / APP_ENV）与默认值。
type LoadOptions struct {
	// ConfigPath 显式指定的基础配置文件路径。
	ConfigPath string
	// Env 显式指定的运行环境，决定叠加哪个 config.<env>.yaml。
	Env string
}

// Config 汇总应用的全部配置项。
type Config struct {
	App   AppConfig   `mapstructure:"app"`
	Log   LogConfig   `mapstructure:"log"`
	MySQL MySQLConfig `mapstructure:"mysql"`
	Redis RedisConfig `mapstructure:"redis"`

	// Env 本次实际生效的环境名。
	Env string `mapstructure:"-"`
	// Sources 本次加载按序合并的配置文件，用于启动日志排查配置来源。
	Sources []string `mapstructure:"-"`
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
	return c.DSNWithParams(nil)
}

// DSNWithParams 在标准连接串基础上追加/覆盖参数。
// 例如迁移进程需要 multiStatements=true 才能在一个脚本里执行多条语句：
//
//	cfg.MySQL.DSNWithParams(map[string]string{"multiStatements": "true"})
func (c MySQLConfig) DSNWithParams(params map[string]string) string {
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
	opts.Timeout = 5 * time.Second
	opts.ReadTimeout = 10 * time.Second
	opts.WriteTimeout = 10 * time.Second

	opts.Params = map[string]string{"charset": charset}
	for k, v := range params {
		opts.Params[k] = v
	}

	return opts.FormatDSN()
}

// Addr 返回 Redis 的 host:port。
func (c RedisConfig) Addr() string {
	return net.JoinHostPort(c.Host, fmt.Sprintf("%d", c.Port))
}

// IsProduction 判断是否运行在生产模式。
func (a AppConfig) IsProduction() bool {
	return strings.EqualFold(a.Env, EnvProduction)
}

// IsDevelopment 判断是否运行在开发模式。
func (a AppConfig) IsDevelopment() bool {
	return strings.EqualFold(a.Env, EnvDevelopment)
}

// LoadConfig 按固定优先级加载配置：
//
//	基础配置 config.yaml
//	  → 环境覆盖 config.<env>.yaml
//	  → 本地覆盖 config.local.yaml
//	  → 环境变量（KOTOBA_ 前缀，点号替换为下划线）
//
// 后加载者覆盖先加载者，最后统一填充默认值并做合法性校验。
func LoadConfig(opts LoadOptions) (*Config, error) {
	rawEnv, env, envExplicit := resolveEnvSelector(opts.Env)

	envForOverlay := env
	if envForOverlay == "" {
		envForOverlay = DefaultEnv
	}

	basePath := resolveConfigPath(opts.ConfigPath)

	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(basePath)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取基础配置文件 %s 失败: %w", basePath, err)
	}
	sources := []string{basePath}

	dir := filepath.Dir(basePath)
	primaryEnvPath := filepath.Join(dir, fmt.Sprintf(EnvConfigNamePattern, envForOverlay))

	// 合并顺序：环境覆盖 → 本地覆盖。两者都是可选文件，不存在则跳过。
	// 环境文件同时兼容 config.prod.yaml 与 config.production.yaml 两种命名。
	overlayPaths := make([]string, 0, 2)
	for _, candidate := range envOverlayPaths(dir, rawEnv, envForOverlay) {
		if fileExists(candidate) {
			overlayPaths = append(overlayPaths, candidate)
			break
		}
	}
	if localPath := filepath.Join(dir, LocalConfigName); fileExists(localPath) {
		overlayPaths = append(overlayPaths, localPath)
	}

	for _, overlay := range overlayPaths {
		v.SetConfigFile(overlay)
		if err := v.MergeInConfig(); err != nil {
			return nil, fmt.Errorf("合并配置文件 %s 失败: %w", overlay, err)
		}
		sources = append(sources, overlay)
	}

	// 环境变量覆盖：为配置结构中的每个字段显式绑定 KOTOBA_<路径>，
	// 这样即使某个字段没有出现在 YAML 里（例如 mysql.password），
	// 也能用 KOTOBA_MYSQL_PASSWORD 提供，容器部署无需改动镜像内的配置文件。
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	bindEnvKeys(v, reflect.TypeOf(Config{}), "")

	cfg := &Config{}
	// 说明：viper 的解码器默认开启 WeaklyTypedInput 并带 StringToSlice 钩子，
	// 因此 "3306" 这类环境变量字符串能正确落到 int 字段，
	// KOTOBA_APP_CORS_ALLOW_ORIGINS="https://a,https://b" 也能落到 []string 字段。
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	cfg.Sources = sources

	// 环境一致性：显式指定的环境必须与配置里的 app.env 相符，
	// 避免「以 production 启动、实际加载 development 配置」这类静默降级。
	switch {
	case envExplicit:
		if cfg.App.Env == "" {
			cfg.App.Env = env
		} else if !strings.EqualFold(cfg.App.Env, env) {
			hint := ""
			if !fileExists(primaryEnvPath) {
				hint = fmt.Sprintf("，且未找到环境配置文件 %s", primaryEnvPath)
			}
			return nil, fmt.Errorf("指定环境 %s 与配置中的 app.env=%s 不一致%s", env, cfg.App.Env, hint)
		}
	case cfg.App.Env == "":
		cfg.App.Env = DefaultEnv
	}
	cfg.App.Env = normalizeEnv(cfg.App.Env)
	cfg.Env = cfg.App.Env

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

// resolveConfigPath 依次尝试显式入参、KOTOBA_CONFIG、默认路径。
func resolveConfigPath(explicit string) string {
	if p := strings.TrimSpace(explicit); p != "" {
		return filepath.Clean(p)
	}
	if p := strings.TrimSpace(os.Getenv(EnvConfigPath)); p != "" {
		return filepath.Clean(p)
	}
	return DefaultConfigPath
}

// resolveEnvSelector 依次尝试显式入参、KOTOBA_ENV、APP_ENV。
// 返回原样的环境名（用于匹配 config.<raw>.yaml）、规范名以及环境是否被显式指定。
func resolveEnvSelector(explicit string) (raw, canonical string, isSet bool) {
	value := strings.TrimSpace(explicit)
	if value == "" {
		for _, key := range []string{EnvAppEnv, EnvAppEnvFallback} {
			if v := strings.TrimSpace(os.Getenv(key)); v != "" {
				value = v
				break
			}
		}
	}
	if value == "" {
		return "", "", false
	}
	return strings.ToLower(value), normalizeEnv(value), true
}

// envOverlayPaths 返回环境覆盖文件的候选路径（按优先级）。
// 同时兼容 config.prod.yaml 与 config.production.yaml 两种命名习惯。
func envOverlayPaths(dir, rawEnv, canonicalEnv string) []string {
	paths := make([]string, 0, 3)
	seen := make(map[string]struct{}, 3)

	for _, name := range []string{rawEnv, canonicalEnv, envAlias(canonicalEnv)} {
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		paths = append(paths, filepath.Join(dir, fmt.Sprintf(EnvConfigNamePattern, name)))
	}

	return paths
}

// envAlias 返回环境名的简写，用于匹配 config.dev.yaml / config.prod.yaml。
func envAlias(env string) string {
	switch env {
	case EnvDevelopment:
		return "dev"
	case EnvProduction:
		return "prod"
	case EnvTest:
		return "test"
	default:
		return env
	}
}

// bindEnvKeys 递归遍历配置结构体，为每个叶子字段绑定同名环境变量，
// 使 KOTOBA_<大写下划线路径> 可以覆盖任意配置项（包括未写入 YAML 的项）。
func bindEnvKeys(v *viper.Viper, t reflect.Type, path string) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		name := field.Tag.Get("mapstructure")
		if name == "-" {
			continue
		}
		if idx := strings.Index(name, ","); idx >= 0 {
			name = name[:idx]
		}
		if name == "" {
			name = strings.ToLower(field.Name)
		}

		key := name
		if path != "" {
			key = path + "." + name
		}

		if field.Type.Kind() == reflect.Struct {
			bindEnvKeys(v, field.Type, key)
			continue
		}

		_ = v.BindEnv(key)
	}
}

// fileExists 判断路径是否为已存在的普通文件。
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// normalizeEnv 归一化环境名，兼容 dev/prod 等简写。
func normalizeEnv(env string) string {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "dev", "develop", "development", "local":
		return EnvDevelopment
	case "prod", "production", "release":
		return EnvProduction
	case "test", "testing":
		return EnvTest
	default:
		return strings.ToLower(strings.TrimSpace(env))
	}
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
	switch c.App.Env {
	case EnvDevelopment, EnvTest, EnvProduction:
	default:
		return fmt.Errorf("app.env 取值非法: %q（可选 %s / %s / %s）",
			c.App.Env, EnvDevelopment, EnvTest, EnvProduction)
	}

	if c.App.Name == "" {
		return errors.New("app.name 不能为空")
	}
	if _, _, err := net.SplitHostPort(c.App.Addr); err != nil {
		return fmt.Errorf("app.addr 格式非法: %q（应形如 :8080 或 127.0.0.1:8080）", c.App.Addr)
	}

	switch strings.ToLower(c.Log.Format) {
	case "console", "json":
	default:
		return fmt.Errorf("log.format 取值非法: %q（可选 console / json）", c.Log.Format)
	}

	// 生产环境必须显式声明来源白名单，防止误放开任意跨域来源。
	if c.App.IsProduction() && len(trimmed(c.App.CORS.AllowOrigins)) == 0 {
		return errors.New("生产环境必须显式配置 app.cors.allow_origins，不允许回显任意来源")
	}

	if c.MySQL.Username == "" {
		return errors.New("mysql.username 不能为空")
	}
	if c.MySQL.Database == "" {
		return errors.New("mysql.database 不能为空")
	}
	if !validPort(c.MySQL.Port) {
		return fmt.Errorf("mysql.port 取值非法: %d", c.MySQL.Port)
	}
	if c.MySQL.MaxIdleConns > c.MySQL.MaxOpenConns {
		return fmt.Errorf("mysql.max_idle_conns(%d) 不能大于 mysql.max_open_conns(%d)",
			c.MySQL.MaxIdleConns, c.MySQL.MaxOpenConns)
	}
	switch strings.ToLower(c.MySQL.LogLevel) {
	case "silent", "error", "warn", "info":
	default:
		return fmt.Errorf("mysql.log_level 取值非法: %s", c.MySQL.LogLevel)
	}

	if !validPort(c.Redis.Port) {
		return fmt.Errorf("redis.port 取值非法: %d", c.Redis.Port)
	}

	return nil
}

func validPort(port int) bool {
	return port > 0 && port <= 65535
}

// trimmed 返回去掉空白项后的副本，用于判断白名单是否实际为空。
func trimmed(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}
