# cloudFile

macOS 桌面文件管理器，用 [Go](https://go.dev) + [mygo](https://github.com/egoist/mygo)（原生 UI，无 webview）编写。设计目标：本地盘之外，可接入 WebDAV 或 S3。

## 功能

- 浏览目录：Finder 式侧边栏（Home/Desktop/Documents/Downloads）、前进/后退、路径显示
- 文件列表：按名称/大小/修改时间排序（点表头）、目录恒排前、过滤搜索框、显示/隐藏点文件
- 文件操作：新建文件夹、就地重命名（Return）、移到废纸篓（右键）、复制/剪切/粘贴（Cmd+C/X/V）
- 打开：双击用系统默认应用打开文件，Quick Look（右键）
- 拖拽：从 Finder 拖文件进窗口导入到当前目录；从列表拖出去到 Finder（复制语义）

## 开发

```sh
cd file-manager
go tool mygo dev      # 开发运行，保存即重建
go test ./...         # 测试（无窗口 UI 测试 + VFS 测试）
go tool mygo build    # 打包 .app 和 dmg 到 build/
```

## 架构

```
vfs/       统一文件系统抽象（vfs.FS 接口 + Local 实现）
main.go    应用入口、窗口创建
model.go   应用状态与文件操作（异步加载、粘贴、导入）
view.go    原生 UI 视图（即时模式：view 是状态的函数）
```

### 接入 WebDAV / S3

实现 `vfs.FS`（`List/Stat/Mkdir/Open/Create/Delete/Rename`），错误尽量映射为 `vfs.ErrNotExist` 等哨兵值，然后在 `model.go` 注册为新位置即可，UI 层完全无感知。`vfs.Local` 可作参考实现。
