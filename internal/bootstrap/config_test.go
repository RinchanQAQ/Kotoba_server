package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const baseConfigYAML = `
app:
  name: kotoba
  env: development
  addr: ":8080"
  cors:
    allow_origins:
      - http://localhost:5173
log:
  level: info
  format: console
mysql:
  host: 127.0.0.1
  port: 3306
  username: root
  database: kotoba
redis:
  host: 127.0.0.1
  port: 6379
`

const prodOverlayYAML = `
app:
  env: production
  cors:
    allow_origins:
      - https://kotoba.example.com
log:
  level: warn
  format: json
mysql:
  max_open_conns: 200
`

// writeFile 在临时目录写入配置片段，便于按需组合多环境场景。
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入 %s 失败: %v", name, err)
	}
	return path
}

// clearEnv 清空可能影响用例的环境变量，保证用例之间互不干扰。
func clearEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{EnvAppEnv, EnvAppEnvFallback, EnvConfigPath, "KOTOBA_MYSQL_PASSWORD"} {
		t.Setenv(key, "")
	}
}

func TestLoadConfigMergesEnvOverlay(t *testing.T) {
	clearEnv(t)

	dir := t.TempDir()
	base := writeFile(t, dir, "config.yaml", baseConfigYAML)
	writeFile(t, dir, "config.prod.yaml", prodOverlayYAML)

	cfg, err := LoadConfig(LoadOptions{ConfigPath: base, Env: "prod"})
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if cfg.Env != EnvProduction || cfg.App.Env != EnvProduction {
		t.Fatalf("环境应为 production，实际 Env=%q app.env=%q", cfg.Env, cfg.App.Env)
	}
	// 环境覆盖文件的字段生效
	if cfg.Log.Format != "json" || cfg.Log.Level != "warn" {
		t.Fatalf("日志配置未被环境文件覆盖: %+v", cfg.Log)
	}
	if cfg.MySQL.MaxOpenConns != 200 {
		t.Fatalf("mysql.max_open_conns 应为 200，实际 %d", cfg.MySQL.MaxOpenConns)
	}
	// 基础配置中未被覆盖的字段保留
	if cfg.MySQL.Port != 3306 || cfg.MySQL.Username != "root" {
		t.Fatalf("基础配置字段丢失: %+v", cfg.MySQL)
	}
	// 秒 -> Duration 换算
	if cfg.App.ReadTimeout != 15*time.Second || cfg.App.ShutdownTimeout != 10*time.Second {
		t.Fatalf("超时换算不正确: %+v", cfg.App)
	}
	// 记录实际参与合并的文件
	if len(cfg.Sources) != 2 {
		t.Fatalf("应合并 2 个文件，实际 %v", cfg.Sources)
	}
}

func TestLoadConfigDefaultsToDevelopment(t *testing.T) {
	clearEnv(t)

	dir := t.TempDir()
	base := writeFile(t, dir, "config.yaml", baseConfigYAML)

	cfg, err := LoadConfig(LoadOptions{ConfigPath: base})
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if cfg.Env != EnvDevelopment {
		t.Fatalf("默认环境应为 development，实际 %q", cfg.Env)
	}
	if cfg.Log.Format != "console" {
		t.Fatalf("开发环境默认日志格式应为 console，实际 %q", cfg.Log.Format)
	}
}

func TestLoadConfigEnvFromEnvironmentVariable(t *testing.T) {
	clearEnv(t)

	dir := t.TempDir()
	base := writeFile(t, dir, "config.yaml", baseConfigYAML)
	writeFile(t, dir, "config.prod.yaml", prodOverlayYAML)

	// KOTOBA_ENV 决定加载哪份环境覆盖文件。
	t.Setenv(EnvAppEnv, "production")

	cfg, err := LoadConfig(LoadOptions{ConfigPath: base})
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if cfg.Env != EnvProduction {
		t.Fatalf("KOTOBA_ENV=production 未生效，实际 %q", cfg.Env)
	}
}

func TestLoadConfigEnvironmentVariableOverride(t *testing.T) {
	clearEnv(t)

	dir := t.TempDir()
	base := writeFile(t, dir, "config.yaml", baseConfigYAML)

	// KOTOBA_<大写下划线路径> 覆盖配置文件中的同名项，主要用于注入密码等敏感信息。
	t.Setenv("KOTOBA_MYSQL_PASSWORD", "from-env")

	cfg, err := LoadConfig(LoadOptions{ConfigPath: base})
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if cfg.MySQL.Password != "from-env" {
		t.Fatalf("环境变量未覆盖 mysql.password，实际 %q", cfg.MySQL.Password)
	}
}

func TestLoadConfigEnvironmentVariableOverrideSlice(t *testing.T) {
	clearEnv(t)

	dir := t.TempDir()
	base := writeFile(t, dir, "config.yaml", baseConfigYAML)

	// 容器里通常用环境变量下发跨域白名单。
	t.Setenv("KOTOBA_APP_CORS_ALLOW_ORIGINS", "https://a.example,https://b.example")

	cfg, err := LoadConfig(LoadOptions{ConfigPath: base})
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	want := []string{"https://a.example", "https://b.example"}
	if len(cfg.App.CORS.AllowOrigins) != len(want) {
		t.Fatalf("跨域白名单应被环境变量替换，实际 %v", cfg.App.CORS.AllowOrigins)
	}
	for i := range want {
		if cfg.App.CORS.AllowOrigins[i] != want[i] {
			t.Fatalf("跨域白名单第 %d 项应为 %q，实际 %q", i, want[i], cfg.App.CORS.AllowOrigins[i])
		}
	}
}

func TestLoadConfigLocalOverlayWins(t *testing.T) {
	clearEnv(t)

	dir := t.TempDir()
	base := writeFile(t, dir, "config.yaml", baseConfigYAML)
	writeFile(t, dir, "config.local.yaml", "mysql:\n  password: local-secret\n")

	cfg, err := LoadConfig(LoadOptions{ConfigPath: base})
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if cfg.MySQL.Password != "local-secret" {
		t.Fatalf("本地覆盖文件未生效，实际 %q", cfg.MySQL.Password)
	}
}

func TestLoadConfigRejectsEnvMismatch(t *testing.T) {
	clearEnv(t)

	dir := t.TempDir()
	// 基础配置声明 development，却要求以 production 启动，且缺少 config.prod.yaml。
	base := writeFile(t, dir, "config.yaml", baseConfigYAML)

	_, err := LoadConfig(LoadOptions{ConfigPath: base, Env: "production"})
	if err == nil {
		t.Fatal("环境与 app.env 不一致时应报错，实际通过")
	}
	if !strings.Contains(err.Error(), "不一致") {
		t.Fatalf("错误信息应说明环境不一致，实际: %v", err)
	}
}

func TestLoadConfigProductionRequiresCORSAllowList(t *testing.T) {
	clearEnv(t)

	dir := t.TempDir()
	base := writeFile(t, dir, "config.yaml", "app:\n  name: kotoba\n  env: production\nmysql:\n  username: root\n  database: kotoba\n")

	_, err := LoadConfig(LoadOptions{ConfigPath: base})
	if err == nil {
		t.Fatal("生产环境缺少 cors.allow_origins 时应报错，实际通过")
	}
	if !strings.Contains(err.Error(), "allow_origins") {
		t.Fatalf("错误信息应指向 cors.allow_origins，实际: %v", err)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	clearEnv(t)

	_, err := LoadConfig(LoadOptions{ConfigPath: filepath.Join(t.TempDir(), "not-exist.yaml")})
	if err == nil {
		t.Fatal("配置文件不存在时应报错，实际通过")
	}
}

func TestNewLoggerRejectsInvalidLevel(t *testing.T) {
	clearEnv(t)

	dir := t.TempDir()
	base := writeFile(t, dir, "config.yaml", baseConfigYAML)

	cfg, err := LoadConfig(LoadOptions{ConfigPath: base})
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	cfg.Log.Level = "verbose"
	if _, err := NewLogger(cfg.Log, cfg.App.Name); err == nil {
		t.Fatal("非法日志级别应报错，实际通过")
	}
}

func TestMySQLConfigDSN(t *testing.T) {
	cfg := MySQLConfig{Host: "127.0.0.1", Port: 3306, Username: "root", Password: "p@ss", Database: "kotoba"}

	dsn := cfg.DSN()
	if !strings.Contains(dsn, "charset=utf8mb4") {
		t.Fatalf("DSN 应包含 charset=utf8mb4: %s", dsn)
	}
	if !strings.Contains(dsn, "parseTime=true") {
		t.Fatalf("DSN 应包含 parseTime=true: %s", dsn)
	}
	if strings.Contains(dsn, "multiStatements") {
		t.Fatalf("业务连接不应开启 multiStatements: %s", dsn)
	}

	// 迁移进程需要一次执行多条语句。
	migrationDSN := cfg.DSNWithParams(map[string]string{"multiStatements": "true"})
	if !strings.Contains(migrationDSN, "multiStatements=true") {
		t.Fatalf("迁移 DSN 应包含 multiStatements=true: %s", migrationDSN)
	}
	if !strings.Contains(migrationDSN, "charset=utf8mb4") {
		t.Fatalf("追加参数不应丢失原有参数: %s", migrationDSN)
	}
}
