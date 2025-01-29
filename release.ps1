Remove-Item "txlogger.exe" -ErrorAction Continue
Remove-Item "install.exe" -ErrorAction SilentlyContinue
Remove-Item "txlogger.zip" -ErrorAction SilentlyContinue
Remove-Item "setup.zip" -ErrorAction SilentlyContinue

.\buildcangw.ps1
.\build.ps1


Write-Host "Building install.exe"

$ifpPath = (Get-Location).Path + "\installer.nsi"
Start-Process -FilePath "C:\Program Files (x86)\NSIS\makensis.exe" -ArgumentList $ifpPath -WorkingDirectory (Get-Location).Path -Wait
if (-not (Test-Path "install.exe")) {
    Write-Host "install.exe not found. Exiting."
    exit
}

$winRarPath = "C:\Program Files\WinRAR\WinRAR.exe"

$filesToAdd = "debug.bat", "libusb-1.0.dll", "txlogger.exe"
$outputZip = "txlogger.zip"
$winRarArgs = "a -m5 -afzip $outputZip $filesToAdd"

Start-Process -FilePath $winRarPath -ArgumentList $winRarArgs -NoNewWindow -Wait

$filesToAdd = "install.exe"
$outputZip = "setup.zip"
$winRarArgs = "a -m5 -afzip $outputZip $filesToAdd"

Start-Process -FilePath $winRarPath -ArgumentList $winRarArgs -NoNewWindow -Wait

Write-Host "Zip files created successfully."

# Invoke-Expression "scp debug.bat libusb-1.0.dll txlogger.exe txlogger.zip setup.zip roffe@roffe.nu:/webroot/roffe/public_html/txlogger"