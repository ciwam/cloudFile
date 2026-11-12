#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"

usage() {
  cat <<'EOF'
构建 cloudFile（Go + mygo 原生 UI）。在 file-manager/ 目录下运行。

用法：
  ./build.sh [选项]

不带参数时只显示本帮助。

选项：
  --no-clean       跳过清理（清理 = go clean -cache + 删除 build/）
  --check          构建前先跑测试、gofmt、go vet、mygo vet
  --platform SPEC  跨平台构建（mygo build -platform 的值原样透传）
                   如 darwin/universal、darwin/amd64,windows/amd64
  --run            打包后启动 .app
  -h, --help       显示本帮助

流程：清理 → 构建（编译 + 打包 .app/.dmg）→ 可选运行。

常用组合：
  ./build.sh --no-clean              快速构建（跳过清理）
  ./build.sh --check                 完整检查后构建
  ./build.sh --no-clean --run        快速构建并启动
  ./build.sh --check --run           检查、构建、启动一条龙
  ./build.sh --platform darwin/universal   跨架构打包

产物在 build/<os>-<arch>/。
EOF
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

if [[ $# -eq 0 ]]; then
  usage
  exit 0
fi

PLATFORM=""
CHECK=false
CLEAN=true
RUN=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --check)    CHECK=true; shift ;;
    --no-clean) CLEAN=false; shift ;;
    --run)      RUN=true; shift ;;
    --platform) PLATFORM="$2"; shift 2 ;;
    *) echo "未知参数: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ "$CLEAN" == true ]]; then
  echo "==> 清理缓存"
  go clean -cache
  rm -rf build
fi

if [[ "$CHECK" == true ]]; then
  echo "==> 测试与静态检查"
  gofmt -l . | grep . && { echo "gofmt 未格式化，见上列文件" >&2; exit 1; }
  go vet ./...
  go tool mygo vet .
  go test -count=1 ./...
fi

echo "==> go tool mygo build"
if [[ -n "$PLATFORM" ]]; then
  go tool mygo build -- -platform "$PLATFORM"
else
  go tool mygo build
fi

echo "==> 完成，产物在 build/："
find build -maxdepth 3 \( -name "*.app" -o -name "*.dmg" -o -type f \) -print 2>/dev/null | sed 's/^/  /'

if [[ "$RUN" == true ]]; then
  APP=$(find build -maxdepth 3 -name "*.app" -type d 2>/dev/null | head -1)
  if [[ -z "$APP" ]]; then
    echo "没找到 .app（--run 只支持本机构建，跨平台产物无法直接运行）" >&2
    exit 1
  fi
  echo "==> 启动 $APP"
  open "$APP"
fi
