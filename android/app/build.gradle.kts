plugins { id("com.android.application"); id("org.jetbrains.kotlin.android") }
val releaseInfo = groovy.json.JsonSlurper().parse(rootProject.file("release.json")) as Map<*, *>
android {
    namespace = "dev.nawa.music"
    compileSdk = 35
    ndkVersion = "28.2.13676358"
    defaultConfig {
        applicationId = "dev.nawa.music"
        minSdk = 28
        targetSdk = 35
        versionCode = (releaseInfo["versionCode"] as Number).toInt()
        versionName = releaseInfo["version"].toString()
        ndk { abiFilters += (project.findProperty("nawaAbis")?.toString() ?: "arm64-v8a").split(",") }
        externalNativeBuild { cmake { arguments += listOf("-DANDROID_STL=c++_shared") } }
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }
    signingConfigs {
        create("release") {
            val key = System.getenv("NAWA_SIGNING_STORE")
            if (key != null) {
                storeFile = file(key)
                storePassword = System.getenv("NAWA_SIGNING_PASSWORD")
                keyAlias = "nawa"
                keyPassword = System.getenv("NAWA_SIGNING_PASSWORD")
            }
        }
    }
    buildTypes {
        release { isMinifyEnabled = false; signingConfig = signingConfigs.getByName("release") }
        debug { applicationIdSuffix = ".debug" }
    }
    compileOptions { sourceCompatibility = JavaVersion.VERSION_17; targetCompatibility = JavaVersion.VERSION_17 }
    kotlinOptions { jvmTarget = "17" }
    externalNativeBuild { cmake { path = file("../native/CMakeLists.txt"); version = "3.22.1" } }
    packaging {
        jniLibs {
            useLegacyPackaging = true
            keepDebugSymbols += "**/*.zip.so"
            val selected = (project.findProperty("nawaAbis")?.toString() ?: "arm64-v8a").split(",")
            listOf("arm64-v8a","armeabi-v7a","x86","x86_64").filter { it !in selected }.forEach { excludes += "**/$it/**" }
        }
        resources.excludes += setOf("META-INF/AL2.0", "META-INF/LGPL2.1")
    }
}
val syncSharedSource = tasks.register<Exec>("syncSharedSource") {
    commandLine(System.getenv("NAWA_PYTHON") ?: "python",rootProject.file("scripts/sync_desktop.py").absolutePath,"--apply")
}
val verifyNativeCore = tasks.register<Exec>("verifyNativeCore") {
    dependsOn(syncSharedSource)
    commandLine(System.getenv("NAWA_PYTHON") ?: "python",rootProject.file("scripts/build_identity.py").absolutePath,"--verify-native",project.findProperty("nawaAbis")?.toString() ?: "arm64-v8a")
}
tasks.named("preBuild").configure { dependsOn(verifyNativeCore) }
dependencies {
    implementation("androidx.activity:activity-ktx:1.10.1")
    implementation("androidx.core:core-ktx:1.16.0")
    implementation("androidx.webkit:webkit:1.13.0")
    implementation("androidx.documentfile:documentfile:1.0.1")
    implementation("androidx.media3:media3-exoplayer:1.6.1")
    implementation("androidx.media3:media3-session:1.6.1")
    implementation("com.google.mlkit:translate:17.0.3")
    implementation("com.google.mlkit:language-id:17.0.6")
    implementation("io.github.junkfood02.youtubedl-android:library:0.18.1")
    implementation("io.github.junkfood02.youtubedl-android:ffmpeg:0.18.1")
    androidTestImplementation("androidx.test:runner:1.6.2")
    androidTestImplementation("androidx.test.ext:junit:1.2.1")
    androidTestImplementation("androidx.test:core:1.6.1")
}
