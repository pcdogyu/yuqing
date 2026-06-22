# 简苏舆情 Android

原生 Android APK 工程，包名 `com.jiansutech.yuqing`。

## 构建

1. 安装 Android Studio、JDK 17 和 Android SDK 35。
2. 用 Android Studio 打开 `android/`。
3. 在 Android Studio 的 Gradle 面板执行 `:app:assembleRelease`；或安装 Gradle 后在 `android/` 下执行 `gradle :app:assembleRelease`。输出位于 `app/build/outputs/apk/release/`。

默认地址面向 Android 模拟器：

- Auth API: `http://10.0.2.2:8081/`
- Content/BFF API: `http://10.0.2.2:8082/`

真机安装时，在登录页修改为局域网或正式服务地址。

## 功能

- 账号登录、Bearer Token 持久化、退出登录。
- 全量门户原生导航：总览、项目、文章、搜索、分析、报告、A 股、研报调研、机构持仓、加密资讯、系统。
- 系统操作通过 `/api/v1/android/actions/{action}` 统一触发，并在 App 内二次确认。
- 低网速下展示最近一次缓存的 Dashboard 数据。
