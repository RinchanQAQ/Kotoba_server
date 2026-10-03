-- 000001 初始表结构（示例）。
--
-- 这是基建自带的示例迁移：给出建表规范（utf8mb4、毫秒精度时间戳、软删除列与索引命名），
-- 真实业务表按同样的命名与版本号规则继续追加 000002_xxx.up.sql / .down.sql 即可。

CREATE TABLE IF NOT EXISTS `users` (
    `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    `username`      VARCHAR(64)     NOT NULL                COMMENT '登录名',
    `email`         VARCHAR(128)    NOT NULL DEFAULT ''     COMMENT '邮箱',
    `password_hash` VARCHAR(255)    NOT NULL DEFAULT ''     COMMENT '密码散列',
    `status`        TINYINT         NOT NULL DEFAULT 1      COMMENT '状态：1 正常，0 禁用',
    `created_at`    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at`    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    `deleted_at`    DATETIME(3)     NULL     DEFAULT NULL   COMMENT '软删除时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_users_username` (`username`),
    KEY `idx_users_email` (`email`),
    KEY `idx_users_deleted_at` (`deleted_at`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_unicode_ci COMMENT = '用户表';
