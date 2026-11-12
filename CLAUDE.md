# cloudFile — macOS 文件管理器

Go + [mygo](https://github.com/egoist/mygo)（原生 UI，package `ui`，无 webview）桌面文件管理器。

## 架构

- `vfs/` — 统一文件系统抽象（`vfs.FS` 接口）。本地盘已有实现 `vfs.Local`；WebDAV/S3 后续实现同一接口即可接入，UI 层不感知后端。
- `main.go` — 应用状态与原生 UI 视图（即时模式：view 是状态的函数，每帧重建）。
- 路径全部为 VFS 路径（`/` 开头、slash 分隔），由后端实现映射到真实地址。

## 常用命令

```sh
cd file-manager
./build.sh --check    # 测试 + vet 通过后打包 .app + dmg 到 build/
./build.sh --platform darwin/universal   # 跨架构（透传给 mygo build）
go tool mygo dev      # 开发（自动重建重启）
go test ./...         # 单元测试（含无窗口 UI 测试）
go test -race ./...   # 并发安全
go tool mygo vet .    # 检查 build 值泄漏
```

## 关键约定（mygo）

- **Element 是 build 值**：绝不存进 struct 字段/goroutine；持久身份用 `ui.Handle`，模型数据放 app 自己的类型。
- **跨 goroutine 改状态**：后台任务结果用 `win.Update(fn)`（主线程执行 + 重绘）发布。
- **列表行内**用 `RowClicked`/`RowDoubleClicked`，不要用 `Clicked`/`Hovered`（会抢走行事件）。
- **系统对话框阻塞**：在 goroutine 里调（`mygo.Dialog.*`）。
- `mygo vet` 必须通过；UI 行为改动要补 `ui.NewTester` 测试。

## 接入新后端（WebDAV / S3）

实现 `vfs.FS` 的 8 个方法（List/Stat/Mkdir/Open/Create/Delete/Rename），错误尽量映射到 `vfs.ErrNotExist` 等哨兵；然后在应用里注册新的位置（见 `main.go` 的 sidebar 位置列表）。
