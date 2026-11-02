$ErrorActionPreference='Stop'
# Run inside Source/translation with a standalone Python 3.12 installation.
$projectRoot=Split-Path $PSScriptRoot -Parent
$releaseRoot=Split-Path $projectRoot -Parent
$environmentPath=Join-Path $PSScriptRoot '.build-env'
python -m venv $environmentPath
$pythonPath=Join-Path $environmentPath 'Scripts/python.exe'
& $pythonPath -m pip install -r (Join-Path $PSScriptRoot 'requirements-pinned.txt')
if($LASTEXITCODE -ne 0){throw 'Dependency installation failed'}
& $pythonPath -m PyInstaller --noconfirm --onedir --console --name MadarTranslate --distpath (Join-Path $PSScriptRoot 'dist') --workpath (Join-Path $PSScriptRoot 'build') --specpath $PSScriptRoot --collect-all ctranslate2 --collect-all tokenizers --collect-all langdetect --exclude-module torch --exclude-module transformers --exclude-module scipy --exclude-module pandas --exclude-module matplotlib (Join-Path $PSScriptRoot 'translate.py')
if($LASTEXITCODE -ne 0){throw 'Worker build failed'}
Write-Host 'Built translator. Copy dist/MadarTranslate contents into the release tools/translator folder. Keep the separately licensed model beside it in tools/models/nllb-int8.'
