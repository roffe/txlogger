# Files to include in the archive
$files = @(
    "debug.bat",
    "libusb-1.0.dll", 
    "txlogger.exe"
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
    $args = "a -afzip `"txlogger.zip`" $($files -join ' ')"
    
    # Execute WinRAR
    Start-Process -FilePath $winrarExe -ArgumentList $args -NoNewWindow -Wait
    
    Write-Host "Archive txlogger.zip created successfully"
} else {
    Write-Error "WinRAR is not installed or not found in expected locations"
}