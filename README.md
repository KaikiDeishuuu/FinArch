# FinArch

工程级可扩展科研经费管理系统（Go + SQLite + Clean Architecture + Wails 示例）。

## 快速开始

```bash
go mod tidy
go run ./cmd/cli init
go run ./cmd/cli seed
go run ./cmd/cli balance
go run ./cmd/cli match 1200 2 6 20
```

## CLI 命令

- `init` 初始化数据库 schema。
- `seed` 写入示例项目与交易。
- `addtx` 新增交易。
- `match` 从未报销个人支出中反推组合。
- `reimburse` 创建报销单并绑定交易（事务）。
- `balance` 计算公司资金池与个人待报销余额。

## 金额约束

所有金额统一使用 `float64` 元（yuan）。

## 事务约束

报销创建使用单事务执行：创建主单 -> 插入明细 -> 标记交易已报销。

## 测试

```bash
go test ./...
```
