$env:GOARCH = "amd64"
$env:CGO_ENABLED = "1" 
$env:GOGC = "100"
$env:CC = "clang.exe"
$env:CXX = "clang.exe"
Invoke-Expression "copy $Env:USERPROFILE\Documents\PlatformIO\Projects\txbridge\.pio\build\esp32dev\firmware.bin .\pkg\ota\"
Invoke-Expression "go generate ./..."
# Invoke-Expression "go run -tags combi . $args"
Invoke-Expression "go run . $args"
#Invoke-Expression "go run . $args"