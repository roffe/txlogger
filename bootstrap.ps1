## Default values ##############################################################
$INSTALLDIR = "C:\"
$LLVMVERSION = "20250114"
$LLVMARCH = "x86_64" # x86_64, aarch64, armv7, i686
$LLVMCLIB = "ucrt" # ucrt, msvcrt
$GOVERSION = "1.23.5"
$GOARCH = "amd64"
$FYNEVERSION = "v2.3.5"

## Ask for user input ##########################################################
$itemp = Read-Host "Enter the installation directory (default: $INSTALLDIR)"
if ($itemp -ne "") {
    $INSTALLDIR = $itemp
}

$itemp = Read-Host "Enter the LLVM version (default: $LLVMVERSION)"
if ($itemp -ne "") {
    $LLVMVERSION = $itemp
}

$itemp = Read-Host "Enter the LLVM architecture [x86_64, aarch64, armv7, i686] (default: $LLVMARCH)"
if ($itemp -ne "") {
    $LLVMARCH = $itemp
}

$itemp = Read-Host "Enter the LLVM C library (default: $LLVMCLIB)"
if ($itemp -ne "") {
    $LLVMCLIB = $itemp
}

$itemp = Read-Host "Enter the Go version (default: $GOVERSION)"
if ($itemp -ne "") {
    $GOVERSION = $itemp
}

$itemp = Read-Host "Enter the Go architecture (default: $GOARCH)"
if ($itemp -ne "") {
    $GOARCH = $itemp
}

$itemp = Read-Host "Enter the Fyne version (default: $FYNEVERSION)"
if ($itemp -ne "") {
    $FYNEVERSION = $itemp
}

## Print summary and ask for confirmation ######################################
Write-Host "## Summary #################################################################"
Write-Host "Installation directory: $INSTALLDIR"
Write-Host "LLVM version: $LLVMVERSION"
Write-Host "LLVM architecture: $LLVMARCH"
Write-Host "LLVM C library: $LLVMCLIB"
Write-Host "Go version: $GOVERSION"
Write-Host "Go architecture: $GOARCH"
Write-Host "Fyne version: $FYNEVERSION"
Write-Host "############################################################################"

$confirmation = Read-Host "Do you want to continue? (Y/N)"
if ($confirmation -ne "Y" -and $confirmation -ne "y") {
    Write-Host "Installation aborted"
    exit
}

## Install Go and llvm-mingw ###################################################
$GOFILENAME = "go$GOVERSION.windows-$GOARCH.zip"
$GOURL = "https://go.dev/dl/${GOFILENAME}"
$GOTEMPFILE = "$env:TEMP\$GOFILENAME"

$LLVMNAME = "llvm-mingw-$LLVMVERSION-$LLVMCLIB-$LLVMARCH"
$LLVMFILENAME = "$LLVMNAME.zip"
$LLVMURL = "https://github.com/mstorsjo/llvm-mingw/releases/download/$LLVMVERSION/$LLVMFILENAME"
$LLVMTEMPFILE = "$env:TEMP\$LLVMFILENAME"

Add-Type -AssemblyName System.IO.Compression.FileSystem

# Courtesy of https://www.techtarget.com/searchitoperations/answer/Manage-the-Windows-PATH-environment-variable-with-PowerShell
Function Set-PathVariable {
    param (
        [string]$AddPath,
        [string]$RemovePath,
        [ValidateSet('Process', 'User', 'Machine')]
        [string]$Scope = 'Process'
    )
    $regexPaths = @()
    if ($PSBoundParameters.Keys -contains 'AddPath') {
        $regexPaths += [regex]::Escape($AddPath)
    }

    if ($PSBoundParameters.Keys -contains 'RemovePath') {
        $regexPaths += [regex]::Escape($RemovePath)
    }
    
    $arrPath = [System.Environment]::GetEnvironmentVariable('PATH', $Scope) -split ';'
    foreach ($path in $regexPaths) {
        $arrPath = $arrPath | Where-Object { $_ -notMatch "^$path\\?" }
    }
    $value = ($arrPath + $addPath) -join ';'
    [System.Environment]::SetEnvironmentVariable('PATH', $value, $Scope)
}

## Install Go ##################################################################
$GODIR = ($INSTALLDIR).Trim("\") + "\go"
if (Test-Path -Path $GODIR) {
    Write-Host "Go is already installed in $GODIR"
}
else {
    Write-Host "Downloading Go from $GOURL to $GOTEMPFILE"
    (New-Object System.Net.WebClient).DownloadFile($GOURL, $GOTEMPFILE)

    Write-Host "Extracting Go to $INSTALLDIR"
    [System.IO.Compression.ZipFile]::ExtractToDirectory($GOTEMPFILE, $INSTALLDIR)

    Write-Host "Removing $GOTEMPFILE"
    Remove-Item $GOTEMPFILE
}

## Install llvm-mingw ###########################################################
$LLVMDIR = ($INSTALLDIR).Trim("\") + "\$LLVMNAME"
if (Test-Path -Path $LLVMDIR) {
    Write-Host "llvm-mingw is already installed in $LLVMDIR"
}
else {
    Write-Host "Downloading llvm-mingw from $LLVMURL to $LLVMTEMPFILE"
    (New-Object System.Net.WebClient).DownloadFile($LLVMURL, $LLVMTEMPFILE)
    
    
    Write-Host "Extracting llvm-mingw to $INSTALLDIR"
    [System.IO.Compression.ZipFile]::ExtractToDirectory($LLVMTEMPFILE, $INSTALLDIR)

    Write-Host "Removing $LLVMTEMPFILE"
    Remove-Item $LLVMTEMPFILE
}

## Set environment variables ###################################################
$GOBINPATH = ($INSTALLDIR).Trim("\") + "\go\bin"
Write-Host "Adding $GOBINPATH to User PATH"
Set-PathVariable -AddPath $GOBINPATH -Scope User
Set-PathVariable -AddPath $GOBINPATH -Scope Process

$GOPATH = "$env:USERPROFILE\go"
Write-Host "Setting GOPATH to $GOPATH"
Set-PathVariable -AddPath $GOPATH -Scope User
Set-PathVariable -AddPath $GOPATH -Scope Process

$GOUSERBINPATH = "$env:USERPROFILE\go\bin"
Write-Host "Adding $GOUSERBINPATH to User PATH"
Set-PathVariable -AddPath "$GOUSERBINPATH" -Scope User
Set-PathVariable -AddPath "$GOUSERBINPATH" -Scope Process

$LLVMBINPATH = ($INSTALLDIR).Trim("\") + "\$LLVMNAME\bin"
Write-Host "Adding $LLVMBINPATH to User PATH"
Set-PathVariable -AddPath $LLVMBINPATH -Scope User
Set-PathVariable -AddPath $LLVMBINPATH -Scope Process

## Install Fyne ################################################################
Write-Host "Installing fyne.io/fyne/v2/cmd/fyne@$FYNEVERSION"
go install "fyne.io/fyne/v2/cmd/fyne@$FYNEVERSION"

## Done ########################################################################
Write-Host "Done, please restart your terminal & IDE to apply the changes"