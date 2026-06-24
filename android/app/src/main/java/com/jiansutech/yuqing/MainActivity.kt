package com.jiansutech.yuqing

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.os.SystemClock
import android.util.Log
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.lifecycle.viewmodel.compose.viewModel
import com.jiansutech.yuqing.ui.YuqingApp
import com.jiansutech.yuqing.ui.YuqingViewModel
import com.jiansutech.yuqing.ui.YuqingViewModelFactory

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        val startedAt = SystemClock.elapsedRealtime()
        Log.i(STARTUP_TAG, "MainActivity.onCreate start savedInstanceState=${savedInstanceState != null}")
        super.onCreate(savedInstanceState)
        requestNotificationPermissionIfNeeded()
        val app = application as YuqingApplication
        setContent {
            Log.i(STARTUP_TAG, "MainActivity.setContent compose tree start")
            val viewModel: YuqingViewModel = viewModel(
                factory = YuqingViewModelFactory(
                    app.sessionStore,
                    app.database.dashboardCacheDao(),
                    app.database.articleUserActionDao(),
                ),
            )
            YuqingApp(viewModel)
        }
        Log.i(STARTUP_TAG, "MainActivity.onCreate end elapsedMs=${SystemClock.elapsedRealtime() - startedAt}")
    }

    override fun onStart() {
        super.onStart()
        Log.i(STARTUP_TAG, "MainActivity.onStart")
    }

    override fun onResume() {
        super.onResume()
        Log.i(STARTUP_TAG, "MainActivity.onResume")
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

}

private const val STARTUP_TAG = "YuqingStartup"
