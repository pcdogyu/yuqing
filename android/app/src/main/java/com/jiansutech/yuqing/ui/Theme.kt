package com.jiansutech.yuqing.ui

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

private val YuqingColors = lightColorScheme(
    primary = Color(0xFF145A3A),
    onPrimary = Color.White,
    secondary = Color(0xFF4A6358),
    tertiary = Color(0xFF7B5734),
    surface = Color(0xFFFDFCF8),
    surfaceVariant = Color(0xFFE5E3DB),
    background = Color(0xFFF7F4EC),
)

@Composable
fun YuqingTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = YuqingColors,
        typography = MaterialTheme.typography,
        content = content,
    )
}
