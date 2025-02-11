Remove-Item "txlogger.exe" -ErrorAction Continue
Remove-Item "install.exe" -ErrorAction SilentlyContinue
Remove-Item "txlogger.zip" -ErrorAction SilentlyContinue
Remove-Item "setup.zip" -ErrorAction SilentlyContinue

.\build_cangateway.ps1
.\build.ps1
.\build_setup.ps1

$winRarPath = "C:\Program Files\WinRAR\WinRAR.exe"

$filesToAdd = "debug.bat", "libusb-1.0.dll", "canlib32.dll", "txlogger.exe", "cangateway.exe"
$outputZip = "txlogger.zip"
$winRarArgs = "a -m5 -afzip $outputZip $filesToAdd"

Start-Process -FilePath $winRarPath -ArgumentList $winRarArgs -NoNewWindow -Wait

$filesToAdd = "install.exe"
$outputZip = "setup.zip"
$winRarArgs = "a -m5 -afzip $outputZip $filesToAdd"

Start-Process -FilePath $winRarPath -ArgumentList $winRarArgs -NoNewWindow -Wait

Write-Host "Zip files created successfully."

scp debug.bat canlib32.dll libusb-1.0.dll canlib32.dll txlogger.exe cangateway.exe txlogger.zip setup.zip roffe@roffe.nu:/webroot/roffe/public_html/txlogger