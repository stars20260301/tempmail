#!/bin/bash
set -e

# 生成 PgBouncer 用户列表（明文密码，用于 scram-sha-256 透传）
PGBOUNCER_AUTH_FILE="/etc/pgbouncer/userlist.txt"
echo "\"${POSTGRES_USER}\" \"${POSTGRES_PASSWORD}\"" > "$PGBOUNCER_AUTH_FILE"

exec pgbouncer /etc/pgbouncer/pgbouncer.ini
