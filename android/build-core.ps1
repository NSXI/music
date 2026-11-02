param([ValidateSet('arm64-v8a','x86_64')][string]$Abi='arm64-v8a')
$ErrorActionPreference='Stop'
$python=if($env:NAWA_PYTHON){$env:NAWA_PYTHON}else{'python'}
& $python (Join-Path $PSScriptRoot 'scripts/sync_desktop.py') --apply
if($LASTEXITCODE -ne 0){throw 'Shared PC/Android source synchronization failed; no stale Android build was produced'}
if (-not $env:ANDROID_HOME) { throw 'Set ANDROID_HOME to the Android SDK directory' }
$compiler=if($Abi -eq 'arm64-v8a'){'aarch64-linux-android28-clang.cmd'}else{'x86_64-linux-android28-clang.cmd'}
$env:GOOS='android'
$env:GOARCH=if($Abi -eq 'arm64-v8a'){'arm64'}else{'amd64'}
$env:CGO_ENABLED='1'
$env:CC=Join-Path $env:ANDROID_HOME "ndk/28.2.13676358/toolchains/llvm/prebuilt/windows-x86_64/bin/$compiler"
$target=Join-Path $PSScriptRoot "app/src/main/jniLibs/$Abi/libnawacore.so"
New-Item -ItemType Directory -Force (Split-Path $target) | Out-Null
& go build -C (Join-Path $PSScriptRoot 'core') -buildmode=c-shared -ldflags '-s -w' -o $target .
if($LASTEXITCODE -ne 0){throw 'Go Android core build failed'}
& $python (Join-Path $PSScriptRoot 'scripts/build_identity.py') --record-native $Abi
if($LASTEXITCODE -ne 0){throw 'Could not record native core source identity'}
