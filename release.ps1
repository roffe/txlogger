param(
    [switch]$beta
)

# Check if WinRAR is installed in common locations
$winrarPaths = @(
    "C:\Program Files\WinRAR\WinRAR.exe",
    "C:\Program Files (x86)\WinRAR\WinRAR.exe"
)

$winrarExe = $null
foreach ($path in $winrarPaths) {
    if (Test-Path $path) {
        $winrarExe = $path
        break
    }
}

New-Item -ItemType Directory -Path "dist" -Force | Out-Null

$outputZip = "dist\txlogger.zip"

if ($beta) {
    $outputZip = "dist\txlogger_beta.zip"
    Remove-Item "txlogger.exe" -ErrorAction Continue
    Remove-Item "txlogger_beta.zip" -ErrorAction SilentlyContinue
    .\build.ps1 -cangateway -txlogger
}
else {
    Remove-Item "txlogger.exe" -ErrorAction Continue
    Remove-Item "txlogger.zip" -ErrorAction SilentlyContinue
    Remove-Item "txlogger_setup.exe" -ErrorAction SilentlyContinue
    Remove-Item "setup.zip" -ErrorAction SilentlyContinue
    .\build.ps1 -release
}

$files = @(
    "debug.bat",
    "vcpkg\packages\libusb_x64-windows\bin\libusb-1.0.dll"
    "canusb\dll64\canusbdrv64.dll"
    "C:\Progra~2\Kvaser\Canlib\Bin\canlib32.dll"
    "cangateway.exe"
    "txlogger.exe"
)

Write-Output "Creating $outputZip"
$winRarArgs = "a -ep -m5 -afzip $outputZip $($files -join ' ')"
Start-Process -FilePath $winrarExe -ArgumentList $winRarArgs -NoNewWindow -Wait


if (-not (Test-Path "txlogger_setup.exe")) {
    Write-Host "txlogger_setup.exe not found. Exiting."
    exit
}
if (-not ($beta)) {
    $filesToAdd = "txlogger_setup.exe"
    $outputZip = "dist\setup.zip"
    $winRarArgs = "a -m5 -afzip $outputZip $filesToAdd"

    Write-Output "Creating setup.zip"
    Start-Process -FilePath $winRarExe -ArgumentList $winRarArgs -NoNewWindow -Wait
}

if ($beta) {
    scp dist\txlogger_beta.zip roffe@192.168.2.177:/var/www/html/txlogger
}
else {
    scp dist\txlogger.zip dist\setup.zip roffe@192.168.2.177:/var/www/html/txlogger
}


