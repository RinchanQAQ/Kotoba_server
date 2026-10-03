package bootstrap

// 配置加载相关常量。
const (
	// DefaultConfigPath 默认的基础配置文件路径，相对于进程工作目录。
	DefaultConfigPath = "configs/config.yaml"

	// EnvConfigNamePattern 环境覆盖文件名模板，例如 config.prod.yaml。
	EnvConfigNamePattern = "config.%s.yaml"

	// LocalConfigName 本地覆盖配置文件名，与基础配置同目录，不纳入版本控制，
	// 用于存放数据库密码等仅本机可见的敏感项。
	LocalConfigName = "config.local.yaml"

	// EnvPrefix 环境变量前缀，例如 KOTOBA_MYSQL_PASSWORD 覆盖 mysql.password。
	EnvPrefix = "KOTOBA"

	// EnvConfigPath 指定配置文件路径的环境变量。
	EnvConfigPath = "KOTOBA_CONFIG"

	// EnvAppEnv 指定运行环境的环境变量。
	EnvAppEnv = "KOTOBA_ENV"

	// EnvAppEnvFallback 兼容业界通用的 APP_ENV 变量。
	EnvAppEnvFallback = "APP_ENV"
)

// 运行环境标识。
const (
	EnvDevelopment = "development"
	EnvTest        = "test"
	EnvProduction  = "production"

	// DefaultEnv 未显式指定环境时使用的默认值。
	DefaultEnv = EnvDevelopment
)
