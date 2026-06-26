import com.android.build.gradle.internal.api.ApkVariantOutputImpl
import java.util.zip.ZipEntry
import java.util.zip.ZipOutputStream

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
    id("org.jetbrains.kotlin.plugin.serialization")
    id("com.google.devtools.ksp")
}

fun gitOutput(vararg args: String): String? {
    return try {
        val process = ProcessBuilder(listOf("git", *args))
            .directory(rootDir)
            .redirectErrorStream(true)
            .start()
        val output = process.inputStream.bufferedReader().use { it.readText().trim() }
        if (process.waitFor() == 0) {
            output.takeIf { it.isNotEmpty() }
        } else {
            null
        }
    } catch (_: Exception) {
        null
    }
}

val commitTimestamp = gitOutput("log", "-1", "--format=%cd", "--date=format:%Y%m%d-%H%M%S")
val commitHash = gitOutput("rev-parse", "--short=8", "HEAD")
val commitVersion = listOfNotNull(commitTimestamp, commitHash).joinToString("-").ifBlank { "unknown" }
val commitCount = gitOutput("rev-list", "--count", "HEAD")?.toIntOrNull()?.coerceAtLeast(1) ?: 1

android {
    namespace = "com.jiansutech.yuqing"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.jiansutech.yuqing"
        minSdk = 26
        targetSdk = 35
        versionCode = commitCount
        versionName = commitVersion

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        buildConfigField("String", "DEFAULT_WEB_BASE_URL", "\"http://yuqin.jiansutech.com:8079/\"")
        buildConfigField("String", "DEFAULT_AUTH_BASE_URL", "\"http://yuqin.jiansutech.com:8081/\"")
        buildConfigField("String", "DEFAULT_API_BASE_URL", "\"http://yuqin.jiansutech.com:8082/\"")
        buildConfigField("String", "DEFAULT_RELEASE_BASE_URL", "\"http://yuqin.jiansutech.com:8099/\"")
    }

    buildTypes {
        release {
            signingConfig = signingConfigs.getByName("debug")
            isMinifyEnabled = false
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }

    applicationVariants.all {
        outputs.all {
            (this as ApkVariantOutputImpl).outputFileName = "yuqing-${commitVersion}-${buildType.name}.apk"
        }
    }
}

val repoReleaseDir = rootProject.layout.projectDirectory.dir("../release")
val releaseApkFileName = "yuqing-${commitVersion}-release.apk"
tasks.register<Copy>("copyReleaseApkToRepoRelease") {
    dependsOn("packageRelease")
    from(layout.buildDirectory.dir("outputs/apk/release")) {
        include("*.apk")
    }
    into(repoReleaseDir)
}
tasks.register("zipReleaseApkToRepoRelease") {
    dependsOn("copyReleaseApkToRepoRelease")
    doLast {
        val apkFile = repoReleaseDir.file(releaseApkFileName).asFile
        require(apkFile.isFile) {
            "Release APK not found: ${apkFile.absolutePath}"
        }

        val zipFile = repoReleaseDir.file(releaseApkFileName.removeSuffix(".apk") + ".zip").asFile
        ZipOutputStream(zipFile.outputStream().buffered()).use { zip ->
            zip.putNextEntry(ZipEntry(apkFile.name))
            apkFile.inputStream().buffered().use { input ->
                input.copyTo(zip)
            }
            zip.closeEntry()
        }
    }
}
afterEvaluate {
    tasks.named("assembleRelease") {
        finalizedBy("zipReleaseApkToRepoRelease")
    }
}

dependencies {
    val composeBom = platform("androidx.compose:compose-bom:2024.12.01")
    implementation(composeBom)
    androidTestImplementation(composeBom)

    implementation("androidx.activity:activity-compose:1.9.3")
    implementation("androidx.core:core-ktx:1.15.0")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.material:material-icons-extended")
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.ui:ui-tooling-preview")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.8.7")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.8.7")
    implementation("androidx.navigation:navigation-compose:2.8.5")
    implementation("androidx.datastore:datastore-preferences:1.1.1")
    implementation("androidx.room:room-runtime:2.6.1")
    implementation("androidx.room:room-ktx:2.6.1")
    ksp("androidx.room:room-compiler:2.6.1")

    implementation("com.squareup.okhttp3:okhttp:4.12.0")
    implementation("com.squareup.okhttp3:logging-interceptor:4.12.0")
    implementation("com.squareup.retrofit2:retrofit:2.11.0")
    implementation("com.jakewharton.retrofit:retrofit2-kotlinx-serialization-converter:1.0.0")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.7.3")
    implementation("com.squareup.okio:okio:3.9.1")

    testImplementation("junit:junit:4.13.2")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.9.0")
    testImplementation("com.squareup.okhttp3:mockwebserver:4.12.0")
    androidTestImplementation("androidx.test.ext:junit:1.2.1")
    androidTestImplementation("androidx.test.espresso:espresso-core:3.6.1")
    androidTestImplementation("androidx.compose.ui:ui-test-junit4")
    debugImplementation("androidx.compose.ui:ui-tooling")
    debugImplementation("androidx.compose.ui:ui-test-manifest")
}
