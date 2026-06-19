package com.jiansutech.yuqing

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
        val app = application as YuqingApplication
        setContent {
            val viewModel: YuqingViewModel = viewModel(
                factory = YuqingViewModelFactory(app.sessionStore, app.database.dashboardCacheDao()),
            )
            YuqingApp(viewModel)
        }
    }
}
