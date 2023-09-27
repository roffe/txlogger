Remove-Item "txlogger.exe" -ErrorAction Continue
Remove-Item "setup.exe" -ErrorAction SilentlyContinue
Remove-Item "txlogger.zip" -ErrorAction SilentlyContinue
Remove-Item "setup.zip" -ErrorAction SilentlyContinue

# Set the environment variables
$env:PKG_CONFIG_PATH = "C:\vcpkg\packages\libusb_x86-windows\lib\pkgconfig"
$env:CGO_CFLAGS = "-IC:\vcpkg\packages\libusb_x86-windows\include\libusb-1.0"
$env:GOARCH = "386"
$env:CGO_ENABLED = "1"

# Execute the command and capture the output
Write-Host "Building txlogger.exe"
Invoke-Expression "fyne package -tags combi --release"

Write-Host "Building setup.exe"

$ifpPath = (Get-Location).Path + "\installer.ifp"
Start-Process -FilePath "C:\Program Files (x86)\solicus\InstallForge\bin\ifbuilderenvx86.exe" -ArgumentList $ifpPath -WorkingDirectory (Get-Location).Path -Wait
if (-not (Test-Path "setup.exe")) {
    Write-Host "setup.exe not found. Exiting."
    exit
}

$winRarPath = "C:\Program Files\WinRAR\WinRAR.exe"

$filesToAdd = "libusb-1.0.dll", "lzhuf.dll", "txlogger.exe"
$outputZip = "txlogger.zip"
$winRarArgs = "a -m5 -afzip $outputZip $filesToAdd"

Start-Process -FilePath $winRarPath -ArgumentList $winRarArgs -NoNewWindow -Wait

$filesToAdd = "setup.exe"
$outputZip = "setup.zip"
$winRarArgs = "a -m5 -afzip $outputZip $filesToAdd"

Start-Process -FilePath $winRarPath -ArgumentList $winRarArgs -NoNewWindow -Wait

Write-Host "Zip files created successfully."

Invoke-Expression "scp libusb-1.0.dll lzhuf.dll txlogger.exe txlogger.zip setup.zip roffe@roffe.nu:/webroot/roffe/public_html/txlogger"