package migrations

import (
	"errors"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// 迁移文件命名规范：000001_init_schema.up.sql
var fileNamePattern = regexp.MustCompile(`^(\d{6})_[a-z0-9_]+\.(up|down)\.sql$`)

// TestMigrationFilesAreValid 校验内嵌迁移脚本的完整性与可解析性，不需要数据库。
func TestMigrationFilesAreValid(t *testing.T) {
	entries, err := files.ReadDir(".")
	if err != nil {
		t.Fatalf("读取内嵌迁移目录失败: %v", err)
	}

	ups := make(map[string]bool)
	downs := make(map[string]bool)

	for _, entry := range entries {
		name := entry.Name()

		matches := fileNamePattern.FindStringSubmatch(name)
		if matches == nil {
			t.Fatalf("迁移文件名不符合规范 <6 位版本号>_<描述>.<up|down>.sql: %s", name)
		}

		if entry.IsDir() {
			t.Fatalf("迁移目录内不应出现子目录: %s", name)
		}

		version := matches[1]
		if matches[2] == "up" {
			ups[version] = true
		} else {
			downs[version] = true
		}
	}

	versions := make([]string, 0, len(ups))
	for version := range ups {
		versions = append(versions, version)
	}
	sort.Strings(versions)

	if len(versions) == 0 {
		t.Fatal("至少需要一个迁移脚本")
	}

	// 每个 up 必须有对应的 down，否则无法回滚。
	for _, version := range versions {
		if !downs[version] {
			t.Fatalf("迁移 %s 缺少对应的 .down.sql 回滚脚本", version)
		}
	}

	// 版本号必须从 000001 开始连续递增。
	for i, version := range versions {
		want := i + 1
		got, err := strconv.Atoi(version)
		if err != nil {
			t.Fatalf("解析版本号 %s 失败: %v", version, err)
		}
		if got != want {
			t.Fatalf("迁移版本号应连续递增，期望 %06d，实际 %s", want, version)
		}
	}
}

// TestEmbeddedSourceIsReadableByMigrate 直接让 golang-migrate 的 iofs 驱动解析内嵌脚本，
// 确保脚本能被真实迁移流程读取（无数据库也能验证）。
func TestEmbeddedSourceIsReadableByMigrate(t *testing.T) {
	src, err := iofs.New(files, ".")
	if err != nil {
		t.Fatalf("构造 iofs 数据源失败: %v", err)
	}
	defer func() { _ = src.Close() }()

	first, err := src.First()
	if err != nil {
		t.Fatalf("读取首个迁移版本失败: %v", err)
	}
	if first != 1 {
		t.Fatalf("首个迁移版本应为 1，实际 %d", first)
	}

	content, err := readUp(src, first)
	if err != nil {
		t.Fatalf("读取迁移脚本内容失败: %v", err)
	}
	if !strings.Contains(content, "CREATE TABLE") {
		t.Fatalf("迁移脚本内容异常: %q", content)
	}

	// 目前只有 000001，再往后应返回 os.ErrNotExist。
	if _, err := src.Next(first); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("已无更多迁移时应返回 os.ErrNotExist，实际 %v", err)
	}
}

func readUp(src source.Driver, version uint) (string, error) {
	// ReadUp 返回 (内容, 标识符, 错误)。
	reader, _, err := src.ReadUp(version)
	if err != nil {
		return "", err
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
