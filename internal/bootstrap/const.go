package bootstrap

// 配置相关常量。
const (
	// DefaultConfigPath 默认配置文件路径，相对于进程工作目录。
	DefaultConfigPath = "configs/config.yaml"

	// LocalConfigName 本地覆盖配置文件名，与主配置同目录，不纳入版本控制。
	LocalConfigName = "config.local.yaml"

	// EnvPrefix 环境变量前缀，例如 KOTOBA_MYSQL_PASSWORD。
	EnvPrefix = "KOTOBA"

	// EnvConfigPath 用于指定配置文件路径的环境变量。
	EnvConfigPath = "KOTOBA_CONFIG"
)