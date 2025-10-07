;--------------------------------
;Include Modern UI

  !include "MUI2.nsh"

;--------------------------------
;General
  !define NAME "txlogger"
  !define REGPATH_UNINSTSUBKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${NAME}"

  ;Name and file
  Name "${NAME}"
  OutFile "setup.exe"
  Unicode True
  RequestExecutionLevel Admin ; Request admin rights on WinVista+ (when UAC is turned on)
  
  ;Default installation folder
  InstallDir "$ProgramFiles\roffe\$(^Name)"
  
  ;Get installation folder from registry if available
  InstallDirRegKey HKLM "${REGPATH_UNINSTSUBKEY}" "UninstallString"

;--------------------------------
;Variables
  Var StartMenuFolder
;--------------------------------
;Interface Settings
  !define MUI_ABORTWARNING
;--------------------------------
;Pages
  ; !insertmacro MUI_PAGE_LICENSE "${NSISDIR}\Docs\Modern UI\License.txt"
  !insertmacro MUI_PAGE_COMPONENTS
  !insertmacro MUI_PAGE_DIRECTORY
  
  ;Start Menu Folder Page Configuration
  !define MUI_STARTMENUPAGE_REGISTRY_ROOT "HKLM" 
  !define MUI_STARTMENUPAGE_REGISTRY_KEY "Software\txlogger" 
  !define MUI_STARTMENUPAGE_REGISTRY_VALUENAME "Start Menu Folder"
  
  !insertmacro MUI_PAGE_STARTMENU Application $StartMenuFolder
  
  !insertmacro MUI_PAGE_INSTFILES
  
  !insertmacro MUI_UNPAGE_CONFIRM
  !insertmacro MUI_UNPAGE_INSTFILES

;--------------------------------
;Languages
  !insertmacro MUI_LANGUAGE "English"
;--------------------------------
;Installer Sections

!macro EnsureAdminRights
  UserInfo::GetAccountType
  Pop $0
  ${If} $0 != "admin" ; Require admin rights on WinNT4+
    MessageBox MB_IconStop "Administrator rights required!"
    SetErrorLevel 740 ; ERROR_ELEVATION_REQUIRED
    Quit
  ${EndIf}
!macroend

Function .onInit
  SetShellVarContext All
  !insertmacro EnsureAdminRights
FunctionEnd

Function un.onInit
  SetShellVarContext All
  !insertmacro EnsureAdminRights
FunctionEnd


Section "core" SecCore
  SetOutPath "$InstDir"
  
  ;ADD YOUR OWN FILES HERE...
  FILE cangateway.exe
  FILE txlogger.exe
  FILE canusbdrv64.dll
  FILE libusb-1.0.dll
  ;FILE canlib32.dll
  FILE debug.bat
  
  ;Store installation folder
  WriteRegStr HKLM "${REGPATH_UNINSTSUBKEY}" "DisplayName" "${NAME}"
  WriteRegStr HKLM "${REGPATH_UNINSTSUBKEY}" "DisplayIcon" "$InstDir\txlogger.exe,0"
  WriteRegStr HKLM "${REGPATH_UNINSTSUBKEY}" "UninstallString" '"$InstDir\Uninstall.exe"'
  WriteRegStr HKLM "${REGPATH_UNINSTSUBKEY}" "QuietUninstallString" '"$InstDir\Uninstall.exe" /S'
  WriteRegDWORD HKLM "${REGPATH_UNINSTSUBKEY}" "NoModify" 1
  WriteRegDWORD HKLM "${REGPATH_UNINSTSUBKEY}" "NoRepair" 1
  
  WriteRegStr HKLM "SOFTWARE\Microsoft\Windows NT\CurrentVersion\AppCompatFlags\Layers" "$InstDir\txlogger.exe" "RUNASADMIN" 
  
  WriteRegStr HKCR "TXLOGGER" '' "txlogger"
  WriteRegStr HKCR "TXLOGGER\DefaultIcon" '' "$InstDir\txlogger.exe"
  WriteRegStr HKCR "TXLOGGER\shell\open\command" '' '"$InstDir\txlogger.exe" -d "$InstDir" "%1"'
  WriteRegStr HKCR "txlogger.exe\shell\open\command" '' '"$InstDir\txlogger.exe" -d "$InstDir" "%1"'
  WriteRegStr HKCR ".t5l" '' "TXLOGGER"
  WriteRegStr HKCR ".t7l" '' "TXLOGGER"
  WriteRegStr HKCR ".t8l" '' "TXLOGGER"

  WriteRegStr HKCU "Software\Classes\Applications\txlogger.exe\shell\open\command" '' '"$InstDir\txlogger.exe" -d "$InstDir" "%1"'

  
  ;Create uninstaller
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  
  !insertmacro MUI_STARTMENU_WRITE_BEGIN Application
    ;Create shortcuts
    CreateDirectory "$SMPROGRAMS\$StartMenuFolder"
    CreateShortcut "$SMPROGRAMS\$StartMenuFolder\txlogger.lnk" "$INSTDIR\txlogger.exe"
    CreateShortcut "$SMPROGRAMS\$StartMenuFolder\Uninstall.lnk" "$INSTDIR\Uninstall.exe"
  !insertmacro MUI_STARTMENU_WRITE_END

SectionEnd

;--------------------------------
;Descriptions

  ;Language strings
  LangString DESC_SecCore ${LANG_ENGLISH} "txlogger main program."

  ;Assign language strings to sections
  !insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
    !insertmacro MUI_DESCRIPTION_TEXT ${SecCore} $(DESC_SecCore)
  !insertmacro MUI_FUNCTION_DESCRIPTION_END
 
;--------------------------------
;Uninstaller Section


!macro DeleteFileOrAskAbort path
  ClearErrors
  Delete "${path}"
  IfErrors 0 +3
    MessageBox MB_ABORTRETRYIGNORE|MB_ICONSTOP 'Unable to delete "${path}"!' IDRETRY -3 IDIGNORE +2
    Abort "Aborted"
!macroend

Section "Uninstall"
  !insertmacro DeleteFileOrAskAbort "$InstDir\txlogger.exe"
  Delete "$InstDir\cangateway.exe"
  Delete "$InstDir\canusbdrv64.dll"
  Delete "$InstDir\libusb-1.0.dll"
  ;Delete "$InstDir\canlib32.dll"
  Delete "$InstDir\debug.bat"
  Delete "$InstDir\Uninstall.exe"
  RMDir "$InstDir"
  

  !insertmacro MUI_STARTMENU_GETFOLDER Application $StartMenuFolder
  Delete "$SMPROGRAMS\$StartMenuFolder\txlogger.lnk"  
  Delete "$SMPROGRAMS\$StartMenuFolder\Uninstall.lnk"
  RMDir "$SMPROGRAMS\$StartMenuFolder"
  
  DeleteRegKey /ifempty HKLM "Software\txlogger"
  DeleteRegKey HKCR "TXLOGGER"
  DeleteRegKey HKCR "txlogger.exe"
  DeleteRegKey HKCR ".t5l"
  DeleteRegKey HKCR ".t7l"
  DeleteRegKey HKCR ".t8l"
  DeleteRegValue HKLM "SOFTWARE\Microsoft\Windows NT\CurrentVersion\AppCompatFlags\Layers" "$InstDir\txlogger.exe"
  DeleteRegKey HKCU "Software\Classes\Applications\txlogger.exe"
SectionEnd