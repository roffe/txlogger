$env:PKG_CONFIG_PATH = "/vcpkg/packages/libusb_x86-windows/lib/pkgconfig"
$env:CGO_CFLAGS = "-I/vcpkg/packages/libusb_x86-windows/include/libusb-1.0"
$env:GOARCH = "386"
$env:CGO_ENABLED = "1"
$env:CC = "C:\\mingw32\\bin\\gcc.exe"
$env:CXX = "C:\\mingw32\\bin\\gcc.exe"

# Invoke-Expression "rsrc -arch 386 -manifest manifest.xml"
Invoke-Expression "copy $Env:USERPROFILE\Documents\PlatformIO\Projects\txbridge\.pio\build\esp32dev\firmware.bin .\pkg\ota\"
Invoke-Expression "go generate ./..."
Invoke-Expression "fyne package -tags combi --release"
# Remove-Item "rsrc_windows_386.syso" -ErrorAction SilentlyContinue

# gtk3_x86-windows
# $env:PKG_CONFIG_PATH = "/vcpkg/packages/libusb_x86-windows/lib/pkgconfig:/vcpkg/packages/gtk3_x86-windows/lib/pkgconfig:/vcpkg/packages/pango_x86-windows/lib/pkgconfig"
# $env:CGO_CFLAGS = "-I/vcpkg/packages/libusb_x86-windows/include/libusb-1.0:/vcpkg/packages/gtk3_x86-windows/include/gtk-3.0:/vcpkg/packages/pango_x86-windows/include/pango-1.0"

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
    $argz = "a -afzip `"txlogger_beta.zip`" $($files -join ' ')"
    
    # Execute WinRAR
    Start-Process -FilePath $winrarExe -ArgumentList $argz -NoNewWindow -Wait
    
    Write-Host "Archive txlogger.zip created successfully"
} else {
    Write-Error "WinRAR is not installed or not found in expected locations"
}

Invoke-Expression "scp txlogger_beta.zip roffe@roffe.nu:/webroot/roffe/public_html/txlogger"
