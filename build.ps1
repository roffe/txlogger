.\build_cangateway.ps1
$env:CGO_FLAGS = ""
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "1"
$env:GOGC = "100"
$env:CC = "clang.exe"
$env:CXX = "clang.exe"
# Invoke-Expression "rsrc -arch 386 -manifest manifest.xml"
Invoke-Expression "copy $Env:USERPROFILE\Documents\PlatformIO\Projects\txbridge\.pio\build\esp32dev\firmware.bin .\pkg\ota\"
go generate ./...
fyne package --release


