// Package migrations 内嵌全部 SQL 迁移脚本，并提供构造 migrate 实例的入口。
//
// 方案：golang-migrate 的版本化 SQL 文件 + go:embed。
//   - 迁移脚本随二进制一起编译，运行环境不需要额外挂载 SQL 目录，容器镜像天然自洽；
//   - 版本状态记录在 schema_migrations 表，重复执行安全（无变更时返回 ErrNoChange）；
//   - up/down 成对出现，支持灰度回滚。
//
// 命名规范：<6 位版本号>_<描述>.up.sql / .down.sql，例如 000002_add_word_table.up.sql。
package migrations

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed *.sql
var files embed.FS

// New 基于给定的数据库连接构造 migrate 实例。
//
// 传入的 sql.DB 需要带 multiStatements=true（见 bootstrap.MySQLConfig.DSNWithParams），
// 否则单个脚本中的多条 SQL 无法执行。
func New(sqlDB *sql.DB) (*migrate.Migrate, error) {
	driver, err := migratemysql.WithInstance(sqlDB, &migratemysql.Config{})
	if err != nil {
		return nil, fmt.Errorf("初始化 migrate MySQL 驱动失败: %w", err)
	}

	src, err := iofs.New(files, ".")
	if err != nil {
		return nil, fmt.Errorf("读取内嵌迁移脚本失败: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "mysql", driver)
	if err != nil {
		return nil, fmt.Errorf("初始化迁移器失败: %w", err)
	}

	return m, nil
}
