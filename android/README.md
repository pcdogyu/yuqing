# 简苏舆情 Android

原生 Android APK 工程，包名 `com.jiansutech.yuqing`。

## 构建

1. 安装 Android Studio、JDK 17 和 Android SDK 35。
2. 用 Android Studio 打开 `android/`。
3. 在 Android Studio 的 Gradle 面板执行 `:app:assembleRelease`；或安装 Gradle 后在 `android/` 下执行 `gradle :app:assembleRelease`。输出位于 `app/build/outputs/apk/release/`。

默认地址指向远端服务：

- Auth API: `http://yuqin.jiansutech.com:8081/`
- Content/BFF API: `http://yuqin.jiansutech.com:8082/`

本地调试时，可在 `app/build.gradle.kts` 中临时改为局域网或模拟器服务地址后重新构建。

## 功能

- 免登录进入原生工作台，默认连接远端 Auth 和 Content/BFF API。
- 全量门户原生导航：总览、项目、文章、搜索、分析、报告、A 股、研报调研、机构持仓、加密资讯、系统。
- A 股推荐通知：每天 09:25、09:30 推送“上午热门股票推荐”，12:55、13:00 推送“下午热门股票推荐”。
- 系统操作通过 `/api/v1/android/actions/{action}` 统一触发，并在 App 内二次确认。
- 低网速下展示最近一次缓存的 Dashboard 数据。
