; Per-user Windows installer for i-mux (D85, T90.8, ARCHITECTURE.md §7.11).
; Cross-built on Linux with makensis (scripts/build-installer.sh).

Unicode True
RequestExecutionLevel user

!ifndef VERSION
  !define VERSION "dev"
!endif
!ifndef ARCH
  !define ARCH "amd64"
!endif
!ifndef BIN_DIR
  !define BIN_DIR "..\bin"
!endif
!ifndef ROOT_DIR
  !define ROOT_DIR ".."
!endif
!ifndef OUT_FILE
  !define OUT_FILE "..\bin\imux_${VERSION}_windows_${ARCH}_setup.exe"
!endif

!include "MUI2.nsh"
!include "WinMessages.nsh"

Name "i-mux ${VERSION}"
OutFile "${OUT_FILE}"
InstallDir "$LOCALAPPDATA\Programs\i-mux"
InstallDirRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\i-mux" "InstallLocation"
BrandingText "i-mux ${VERSION}"

!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\i-mux"

!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Section "Install"
  SetShellVarContext current
  SetOutPath "$INSTDIR"

  ; Move any running executables to *.old so an in-place upgrade succeeds while
  ; a session or sidebar is open; imux removes *.old on the next start.
  Delete "$INSTDIR\imux.exe.old"
  Delete "$INSTDIR\imux-core.exe.old"
  IfFileExists "$INSTDIR\imux.exe" 0 +2
    Rename "$INSTDIR\imux.exe" "$INSTDIR\imux.exe.old"
  IfFileExists "$INSTDIR\imux-core.exe" 0 +2
    Rename "$INSTDIR\imux-core.exe" "$INSTDIR\imux-core.exe.old"

  File "${BIN_DIR}\imux.exe"
  File "${BIN_DIR}\imux-core.exe"
  File "${ROOT_DIR}\LICENSE"
  File "${ROOT_DIR}\README.md"

  Delete "$INSTDIR\imux.exe.old"
  Delete "$INSTDIR\imux-core.exe.old"

  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; Start menu entry (starting in %USERPROFILE% so imux opens the project
  ; picker): Windows Terminal when present, else conhost (imux.exe directly).
  SetOutPath "$PROFILE"
  IfFileExists "$LOCALAPPDATA\Microsoft\WindowsApps\wt.exe" 0 lbl_conhost
    CreateShortCut "$SMPROGRAMS\i-mux.lnk" "$LOCALAPPDATA\Microsoft\WindowsApps\wt.exe" '-d "$PROFILE" "$INSTDIR\imux.exe"' "$INSTDIR\imux.exe" 0
    Goto lbl_shortcut_done
  lbl_conhost:
    CreateShortCut "$SMPROGRAMS\i-mux.lnk" "$INSTDIR\imux.exe" "" "$INSTDIR\imux.exe" 0
  lbl_shortcut_done:
  SetOutPath "$INSTDIR"

  ; Per-user uninstaller in Apps & features.
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "i-mux"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "hamzayogurtcuoglu"
  WriteRegStr HKCU "${UNINST_KEY}" "URLInfoAbout" "https://github.com/hamzayogurtcuoglu/i-mux"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\imux.exe"
  WriteRegStr HKCU "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKCU "${UNINST_KEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1

  ; Add $INSTDIR to the user's PATH without NSIS's 1024-byte string cap and
  ; without expanding existing %VAR% entries in HKCU\Environment\Path.
  System::Call 'kernel32::SetEnvironmentVariable(t "IMUX_INSTDIR", t "$INSTDIR")'
  nsExec::ExecToLog `"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -WindowStyle Hidden -Command "$$d = $$env:IMUX_INSTDIR; $$k = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $$true); if ($$null -ne $$k) { $$cur = [string]$$k.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames); $$parts = @($$cur -split ';' | Where-Object { $$_ }); if (-not ($$parts | Where-Object { $$_.TrimEnd('\') -ieq $$d.TrimEnd('\') })) { $$new = ($$parts + $$d) -join ';'; $$k.SetValue('Path', $$new, [Microsoft.Win32.RegistryValueKind]::ExpandString) }; $$k.Close() }"`
  SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=500
SectionEnd

Section "Uninstall"
  SetShellVarContext current

  Delete "$INSTDIR\imux.exe"
  Delete "$INSTDIR\imux-core.exe"
  Delete "$INSTDIR\imux.exe.old"
  Delete "$INSTDIR\imux-core.exe.old"
  Delete "$INSTDIR\LICENSE"
  Delete "$INSTDIR\README.md"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"

  Delete "$SMPROGRAMS\i-mux.lnk"
  DeleteRegKey HKCU "${UNINST_KEY}"

  ; Remove $INSTDIR from the user's PATH.
  System::Call 'kernel32::SetEnvironmentVariable(t "IMUX_INSTDIR", t "$INSTDIR")'
  nsExec::ExecToLog `"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -WindowStyle Hidden -Command "$$d = $$env:IMUX_INSTDIR; $$k = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $$true); if ($$null -ne $$k) { $$cur = [string]$$k.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames); $$parts = @($$cur -split ';' | Where-Object { $$_ -and ($$_.TrimEnd('\') -ine $$d.TrimEnd('\')) }); $$k.SetValue('Path', ($$parts -join ';'), [Microsoft.Win32.RegistryValueKind]::ExpandString); $$k.Close() }"`
  SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=500
SectionEnd
