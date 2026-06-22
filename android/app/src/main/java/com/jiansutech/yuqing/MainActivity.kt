package com.jiansutech.yuqing

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.lifecycle.viewmodel.compose.viewModel
import com.jiansutech.yuqing.ui.YuqingApp
import com.jiansutech.yuqing.ui.YuqingViewModel
import com.jiansutech.yuqing.ui.YuqingViewModelFactory

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        requestNotificationPermissionIfNeeded()
        val app = application as YuqingApplication
        setContent {
            val viewModel: YuqingViewModel = viewModel(
                factory = YuqingViewModelFactory(app.sessionStore, app.database.dashboardCacheDao()),
            )
            YuqingApp(viewModel)
        }
    }

    private fun requestNotificationPermissionIfNeeded() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 1001)
        }
    }
}
