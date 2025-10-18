$temp_dir = ".\setup_temp"
$canusb = "https://www.canusb.com/files/canusb_dll_driver.zip"
$canlib = "https://pim.kvaser.com/var/assets/Product_Resources/canlib.exe"
$llvm = "https://github.com/mstorsjo/llvm-mingw/releases/download/20251007/llvm-mingw-20251007-ucrt-x86_64.zip"

# create directory temp if not existing
if (-not (Test-Path -Path "$temp_dir")) {
    Write-Output "Creating temporary directory..."
    New-Item -ItemType Directory -Path "$temp_dir" | Out-Null
}

# create directory canusb if not existing
if (-not (Test-Path -Path ".\canusb")) {
    Write-Output "Creating canusb directory..."
    New-Item -ItemType Directory -Path ".\canusb" | Out-Null
}

# download canusb driver to temp folder
Write-Output "Downloading CANUSB SDK..."
Invoke-WebRequest -Uri $canusb -OutFile "$temp_dir\canusb_dll_driver.zip"

# download canlib installer to temp folder
Write-Output "Downloading CANLIB installer..."
Invoke-WebRequest -Uri $canlib -OutFile "$temp_dir\canlib.exe"

# extract canusb driver
Write-Output "Extracting CANUSB"
Expand-Archive -Path "$temp_dir\canusb_dll_driver.zip" -DestinationPath ".\canusb" -Force

# download llvm-mingw
Write-Output "Downloading llvm-MinGW"
Invoke-WebRequest -Uri $llvm -OutFile "$temp_dir\llvm-mingw.zip"

# Write-Output "Extracting llvm-MinGW"
Expand-Archive -Path "$temp_dir\llvm-mingw.zip" -DestinationPath ".\" -Force

# rename folder llvm-mingw-20251007-ucrt-x86_64 to llvm-mingw
Write-Output "Renaming llvm-MinGW folder"
Rename-Item -Path ".\llvm-mingw-20251007-ucrt-x86_64" -NewName "llvm-mingw"

Write-Output "Installing CANLIB"
Start-Process -FilePath "$temp_dir\canlib.exe" -ArgumentList "/S" -Wait

Write-Output "Setting up vcpkg"
if (-not (Test-Path -Path ".\vcpkg")) {
    git clone https://github.com/microsoft/vcpkg.git --depth=1
}

Write-Output "Bootstrapping vcpkg"
.\vcpkg\bootstrap-vcpkg.bat -disableMetrics

Write-Output "Installing libusb"
.\vcpkg\vcpkg.exe install 'libusb:x64-windows'
# .\vcpkg\vcpkg.exe install 'libusb:x86-windows'

Write-Output "Cleaning up"
Remove-Item -Recurse -Force -Path $temp_dir