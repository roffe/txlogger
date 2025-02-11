.\build_cangateway.ps1
.\build.ps1
# Files to include in the archive
$files = @(
    "debug.bat",
    "libusb-1.0.dll", 
    "canlib32.dll",
    "txlogger.exe"
    "cangateway.exe"
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

if ($winrarExe) {
    # Create archive command
    $argz = "a -afzip `"txlogger_beta.zip`" $($files -join ' ')"
    
    # Execute WinRAR
    Start-Process -FilePath $winrarExe -ArgumentList $argz -NoNewWindow -Wait
    
    Write-Host "Archive txlogger.zip created successfully"
}
else {
    Write-Error "WinRAR is not installed or not found in expected locations"
}

scp txlogger_beta.zip roffe@roffe.nu:/webroot/roffe/public_html/txlogger
