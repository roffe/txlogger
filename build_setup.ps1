$ifpPath = (Get-Location).Path + "\installer.nsi"

Start-Process -FilePath "C:\Program Files (x86)\NSIS\makensis.exe" -ArgumentList $ifpPath -WorkingDirectory (Get-Location).Path -Wait

if (-not (Test-Path "install.exe")) {
    Write-Host "install.exe not found. Exiting."
    exit
}

$winRarPath = "C:\Program Files\WinRAR\WinRAR.exe"
$filesToAdd = "install.exe"
$outputZip = "setup.zip"
$winRarArgs = "a -m5 -afzip $outputZip $filesToAdd"

Start-Process -FilePath $winRarPath -ArgumentList $winRarArgs -NoNewWindow -Wait