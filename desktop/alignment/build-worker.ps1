$ErrorActionPreference='Stop'
$environmentPath=Join-Path $PSScriptRoot '.build-env'
python -m venv $environmentPath
$pythonPath=Join-Path $environmentPath 'Scripts/python.exe'
& $pythonPath -m pip install -r (Join-Path $PSScriptRoot 'requirements-pinned.txt') --extra-index-url https://download.pytorch.org/whl/cpu
if($LASTEXITCODE -ne 0){throw 'Dependency installation failed'}
$env:NUMBA_DISABLE_JIT='1'
& $pythonPath -m PyInstaller --noconfirm --onedir --console --name MadarAlignV2 --distpath (Join-Path $PSScriptRoot 'dist') --workpath (Join-Path $PSScriptRoot 'build') --specpath $PSScriptRoot --collect-all whisper --collect-all stable_whisper --collect-all torchaudio --hidden-import tiktoken_ext.openai_public --exclude-module matplotlib --exclude-module pandas --exclude-module transformers --exclude-module faster_whisper --exclude-module demucs (Join-Path $PSScriptRoot 'align.py')
if($LASTEXITCODE -ne 0){throw 'Worker build failed'}
Write-Host 'Built aligner. Copy dist/MadarAlignV2 contents into tools/aligner. Keep the official Whisper base.pt in tools/models.'
