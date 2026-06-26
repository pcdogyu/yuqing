package com.jiansutech.yuqing

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.os.SystemClock
import android.util.Log
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.viewmodel.compose.viewModel
import com.jiansutech.yuqing.data.AppInstallStore
import com.jiansutech.yuqing.data.ReleaseUpdater
import com.jiansutech.yuqing.data.ReleaseUpgradePolicy
import com.jiansutech.yuqing.data.isInstallRecordExpired
import com.jiansutech.yuqing.ui.VersionUpgradeStatus
import com.jiansutech.yuqing.ui.VersionUpgradeUiState
import com.jiansutech.yuqing.ui.YuqingApp
import com.jiansutech.yuqing.ui.YuqingViewModel
import com.jiansutech.yuqing.ui.YuqingViewModelFactory
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

private data class AppUpgradeUiState(
    val required: Boolean = false,
    val loading: Boolean = false,
    val message: String = "",
)

class MainActivity : ComponentActivity() {
    private lateinit var installStore: AppInstallStore
    private lateinit var releaseUpdater: ReleaseUpdater
    private val upgradeState = MutableStateFlow(AppUpgradeUiState())
    private val versionUpgradeState = MutableStateFlow(VersionUpgradeUiState())

    override fun onCreate(savedInstanceState: Bundle?) {
        val startedAt = SystemClock.elapsedRealtime()
        Log.i(STARTUP_TAG, "MainActivity.onCreate start savedInstanceState=${savedInstanceState != null}")
        super.onCreate(savedInstanceState)
        installStore = AppInstallStore(this)
        releaseUpdater = ReleaseUpdater(this)
        requestNotificationPermissionIfNeeded()
        val app = application as YuqingApplication
        setContent {
            Log.i(STARTUP_TAG, "MainActivity.setContent compose tree start")
            val upgrade by upgradeState.collectAsState()
            val viewModel: YuqingViewModel = viewModel(
                factory = YuqingViewModelFactory(
                    app.sessionStore,
                    app.database.dashboardCacheDao(),
                    app.database.articleUserActionDao(),
                ),
            )
            val versionUpgrade by versionUpgradeState.collectAsState()
            YuqingApp(
                viewModel = viewModel,
                versionUpgradeState = versionUpgrade,
                onCheckUpgrade = ::checkVersionUpgrade,
            )
            if (upgrade.required) {
                ForceUpgradeDialog(
                    state = upgrade,
                    onUpgrade = ::startUpgradeDownload,
                    onCancel = ::exitAppForUpgradeCancel,
                )
            }
        }
        checkAppUsageExpiry()
        Log.i(STARTUP_TAG, "MainActivity.onCreate end elapsedMs=${SystemClock.elapsedRealtime() - startedAt}")
    }

    override fun onStart() {
        super.onStart()
        Log.i(STARTUP_TAG, "MainActivity.onStart")
    }

    override fun onResume() {
        super.onResume()
        Log.i(STARTUP_TAG, "MainActivity.onResume")
        checkAppUsageExpiry()
    }

    override fun onPause() {
        Log.i(STARTUP_TAG, "MainActivity.onPause")
        super.onPause()
    }

    override fun onStop() {
        Log.i(STARTUP_TAG, "MainActivity.onStop")
        super.onStop()
    }

    override fun onDestroy() {
        Log.i(STARTUP_TAG, "MainActivity.onDestroy")
        super.onDestroy()
    }

    private fun requestNotificationPermissionIfNeeded() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            val granted = checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED
            Log.i(STARTUP_TAG, "MainActivity.notificationPermission granted=$granted")
            if (!granted) {
                Log.i(STARTUP_TAG, "MainActivity.requestNotificationPermission")
                requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 1001)
            }
        }
    }

    private fun checkAppUsageExpiry() {
        lifecycleScope.launch {
            runCatching {
                val record = installStore.ensureCurrentVersionInstallRecord(
                    currentVersionCode = BuildConfig.VERSION_CODE,
                    currentVersionName = BuildConfig.VERSION_NAME,
                )
                val expired = isInstallRecordExpired(record.installedAtMillis, System.currentTimeMillis())
                upgradeState.update {
                    if (expired) {
                        it.copy(required = true, message = "当前安装版本已使用超过6个月，请升级到最新版本。")
                    } else {
                        it.copy(required = false, message = "")
                    }
                }
            }.onFailure { throwable ->
                Log.w(STARTUP_TAG, "MainActivity.install expiry check skipped", throwable)
            }
        }
    }

    private fun startUpgradeDownload() {
        if (upgradeState.value.loading) {
            return
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O && !packageManager.canRequestPackageInstalls()) {
            upgradeState.update {
                it.copy(message = "请先允许安装未知来源应用，授权后返回并重新点击升级。")
            }
            startActivity(releaseUpdater.unknownSourcesSettingsIntent())
            return
        }
        lifecycleScope.launch {
            upgradeState.update { it.copy(loading = true, message = "正在获取最新安装包...") }
            runCatching {
                val source = releaseUpdater.selectReleaseSource()
                upgradeState.update { it.copy(message = "使用${source.label}发布服务，正在下载最新安装包...") }
                val latest = releaseUpdater.fetchLatest(source.baseUrl)
                val file = releaseUpdater.downloadApk(latest, source.baseUrl, preferBaseDownloadUrl = true)
                startActivity(releaseUpdater.installApk(file))
                upgradeState.update { it.copy(loading = false, message = "安装器已打开，请完成升级。") }
            }.onFailure { throwable ->
                Log.e(STARTUP_TAG, "MainActivity.release download failed", throwable)
                upgradeState.update {
                    it.copy(
                        loading = false,
                        message = throwable.message ?: "升级失败，请检查网络后重试。",
                    )
                }
            }
        }
    }

    private fun checkVersionUpgrade() {
        if (versionUpgradeState.value.status == VersionUpgradeStatus.Checking ||
            versionUpgradeState.value.status == VersionUpgradeStatus.Downloading
        ) {
            return
        }
        lifecycleScope.launch {
            versionUpgradeState.update {
                VersionUpgradeUiState(
                    status = VersionUpgradeStatus.Checking,
                    message = "正在检测内网发布服务...",
                )
            }
            runCatching {
                val source = releaseUpdater.selectReleaseSource()
                versionUpgradeState.update {
                    it.copy(
                        sourceLabel = source.label,
                        status = VersionUpgradeStatus.Checking,
                        message = "使用${source.label}发布服务，正在获取版本...",
                    )
                }
                val latest = releaseUpdater.fetchLatest(source.baseUrl)
                val latestName = latest.versionName.ifBlank { latest.fileName.ifBlank { "--" } }
                if (ReleaseUpgradePolicy.isCurrentVersion(latest, BuildConfig.VERSION_NAME, BuildConfig.VERSION_CODE)) {
                    versionUpgradeState.update {
                        it.copy(
                            status = VersionUpgradeStatus.Latest,
                            latestVersionName = latestName,
                            latestFileName = latest.fileName,
                            message = "当前已是最新版本。",
                        )
                    }
                    return@launch
                }
                versionUpgradeState.update {
                    it.copy(
                        status = VersionUpgradeStatus.UpdateFound,
                        latestVersionName = latestName,
                        latestFileName = latest.fileName,
                        message = "发现新版本 $latestName，准备下载。",
                    )
                }
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O && !packageManager.canRequestPackageInstalls()) {
                    versionUpgradeState.update {
                        it.copy(
                            status = VersionUpgradeStatus.UpdateFound,
                            message = "发现新版本 $latestName，请先允许安装未知来源应用，授权后重新点击检测升级。",
                        )
                    }
                    startActivity(releaseUpdater.unknownSourcesSettingsIntent())
                    return@launch
                }
                versionUpgradeState.update {
                    it.copy(
                        status = VersionUpgradeStatus.Downloading,
                        message = "发现新版本 $latestName，正在下载...",
                    )
                }
                val file = releaseUpdater.downloadApk(latest, source.baseUrl, preferBaseDownloadUrl = true)
                startActivity(releaseUpdater.installApk(file))
                versionUpgradeState.update {
                    it.copy(
                        status = VersionUpgradeStatus.InstallerOpened,
                        message = "安装器已打开，请完成升级。",
                    )
                }
            }.onFailure { throwable ->
                Log.e(STARTUP_TAG, "MainActivity.manual release check failed", throwable)
                versionUpgradeState.update {
                    it.copy(
                        status = VersionUpgradeStatus.Failed,
                        message = throwable.message ?: "检测升级失败，请检查网络后重试。",
                    )
                }
            }
        }
    }

    private fun exitAppForUpgradeCancel() {
        finishAffinity()
    }
}

@Composable
private fun ForceUpgradeDialog(
    state: AppUpgradeUiState,
    onUpgrade: () -> Unit,
    onCancel: () -> Unit,
) {
    BackHandler(onBack = onCancel)
    AlertDialog(
        onDismissRequest = {},
        title = { Text("需要升级") },
        text = { Text(state.message.ifBlank { "当前安装版本已超过最大使用周期，请升级到最新版本。" }) },
        confirmButton = {
            Button(onClick = onUpgrade, enabled = !state.loading) {
                Text(if (state.loading) "升级中..." else "立即升级")
            }
        },
        dismissButton = {
            TextButton(onClick = onCancel, enabled = !state.loading) {
                Text("取消")
            }
        },
    )
}

private const val STARTUP_TAG = "YuqingStartup"
