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

$outputZip = "txlogger.zip"

if ($beta) {
    $outputZip = "txlogger_beta.zip"
    Remove-Item "txlogger.exe" -ErrorAction Continue
    Remove-Item "txlogger_beta.zip" -ErrorAction SilentlyContinue
    .\build.ps1 -cangateway -txlogger
}
else {
    Remove-Item "txlogger.exe" -ErrorAction Continue
    Remove-Item "txlogger.zip" -ErrorAction SilentlyContinue
    Remove-Item "setup.exe" -ErrorAction SilentlyContinue
    Remove-Item "setup.zip" -ErrorAction SilentlyContinue
    .\build.ps1 -cangateway -txlogger -setup
}

$files = @(
    "debug.bat",
    "vcpkg\packages\libusb_x64-windows\bin\libusb-1.0.dll"
    "canusb\dll64\canusbdrv64.dll"
    "cangateway.exe"
    "txlogger.exe"
)

Write-Output "Creating $outputZip"
$winRarArgs = "a -ep -m5 -afzip $outputZip $($files -join ' ')"
Start-Process -FilePath $winrarExe -ArgumentList $winRarArgs -NoNewWindow -Wait

if ($beta) {
    scp txlogger_beta.zip roffe@roffe.nu:/webroot/roffe/public_html/txlogger
}
else {
    scp txlogger.zip setup.zip roffe@roffe.nu:/webroot/roffe/public_html/txlogger
}


