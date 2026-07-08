# Android APK 发布流程

本文档是后续发布 Android APK 到 `http://10.15.0.7:8099/release/` 的固定流程。

## 发布服务器

- 操作系统：Windows 11
- 共享目录：`\\10.15.0.7\yuqing-release`
- 发布页面：`http://10.15.0.7:8099/release/`
- 最新版本 API：`http://10.15.0.7:8099/api/v1/release/latest`
- 本地凭据文件：`release\netuser.json`

`release\netuser.json` 保存 Windows 共享目录登录信息，仅用于本机发布，不提交到 Git。

## 准备

1. 确认需要发布的代码已经提交并推送。
2. 确认工作区没有会影响 Android 构建的未提交改动。
3. 确认 `release\netuser.json` 存在。

检查命令：

```powershell
git status --short
git branch -vv
git log -1 --oneline
```

## 构建并发布

从仓库根目录执行：

```powershell
.\scripts\publish-android-release.ps1
```

脚本会执行：

1. `gradle :app:assembleRelease`
2. 将 release APK/ZIP 复制到本地 `release\`
3. 校验 APK 签名
4. 读取 `release\netuser.json`
5. 使用 `net use` 连接 `\\10.15.0.7\yuqing-release`
6. 使用 `robocopy` 同步 `*.apk` 和 `*.zip`
7. 调用 `api/v1/release/latest` 回读最新发布结果

只复制已经构建好的本地 release 包时：

```powershell
.\scripts\publish-android-release.ps1 -SkipBuild
```

只构建本地 release 包、不复制到服务器时：

```powershell
.\scripts\publish-android-release.ps1 -SkipCopy
```

## 验证

发布后必须确认 latest API 指向新 APK：

```powershell
Invoke-RestMethod -UseBasicParsing `
  -Uri http://10.15.0.7:8099/api/v1/release/latest `
  -TimeoutSec 10 |
  ConvertTo-Json -Depth 8
```

同时确认发布页面包含新 APK 和 ZIP：

```powershell
Invoke-WebRequest -UseBasicParsing `
  -Uri http://10.15.0.7:8099/release/ `
  -TimeoutSec 10 |
  Select-Object -ExpandProperty Content
```

成功标准：

- `release-service /healthz` 返回正常。
- `api/v1/release/latest` 的 `file_name` 是本次新 APK。
- `download_url` 指向 `http://10.15.0.7:8099/release/<apk文件名>`。
- `/release/` 页面能看到新 APK 和 ZIP。

## 发布服务

如果 `8099` 不通，需要在发布服务器上启动 release service。服务环境变量：

```powershell
$env:YUQING_RELEASE_ADDR=':8099'
$env:YUQING_RELEASE_DIR='C:\yuqing\release'
$env:YUQING_RELEASE_URL='http://10.15.0.7:8099'
Set-Location C:\yuqing\go
.\run.bat --skip-pull
```

健康检查：

```powershell
Invoke-RestMethod -UseBasicParsing `
  -Uri http://10.15.0.7:8099/healthz `
  -TimeoutSec 10
```
