!include "x64.nsh"
!include "WinVer.nsh"
!include "FileFunc.nsh"

!ifndef INFO_PROJECTNAME
  !define INFO_PROJECTNAME "termizard"
!endif
!ifndef INFO_COMPANYNAME
  !define INFO_COMPANYNAME "termizard"
!endif
!ifndef INFO_PRODUCTNAME
  !define INFO_PRODUCTNAME "termizard"
!endif
!ifndef INFO_PRODUCTVERSION
  !define INFO_PRODUCTVERSION "0.1.0"
!endif
!ifndef INFO_COPYRIGHT
  !define INFO_COPYRIGHT "© 2026, termizard contributors"
!endif
!ifndef PRODUCT_EXECUTABLE
  !define PRODUCT_EXECUTABLE "${INFO_PROJECTNAME}.exe"
!endif
!ifndef UNINST_KEY_NAME
  !define UNINST_KEY_NAME "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
!endif
!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${UNINST_KEY_NAME}"

!ifndef INSTALL_SCOPE
  !define INSTALL_SCOPE "machine"
!endif

!ifndef REQUEST_EXECUTION_LEVEL
  !if "${INSTALL_SCOPE}" == "user"
    !define REQUEST_EXECUTION_LEVEL "user"
  !else
    !define REQUEST_EXECUTION_LEVEL "admin"
  !endif
!endif

RequestExecutionLevel "${REQUEST_EXECUTION_LEVEL}"

!ifdef ARG_AMD64_BINARY
  !define SUPPORTS_AMD64
!endif

!ifdef ARG_ARM64_BINARY
  !define SUPPORTS_ARM64
!endif

!ifdef SUPPORTS_AMD64
  !ifdef SUPPORTS_ARM64
    !define ARCH "amd64_arm64"
  !else
    !define ARCH "amd64"
  !endif
!else
  !ifdef SUPPORTS_ARM64
    !define ARCH "arm64"
  !else
    !error "Undefined ARCH: provide ARG_AMD64_BINARY and/or ARG_ARM64_BINARY"
  !endif
!endif

!macro termizard.checkArchitecture
  !ifndef TERMIZARD_WIN10_REQUIRED
    !define TERMIZARD_WIN10_REQUIRED "termizard requires Windows 10 or later."
  !endif
  !ifndef TERMIZARD_ARCH_NOT_SUPPORTED
    !define TERMIZARD_ARCH_NOT_SUPPORTED "termizard cannot be installed on this CPU architecture. Supports: ${ARCH}"
  !endif

  ${If} ${AtLeastWin10}
    !ifdef SUPPORTS_AMD64
      ${If} ${IsNativeAMD64}
        Goto ok
      ${EndIf}
    !endif
    !ifdef SUPPORTS_ARM64
      ${If} ${IsNativeARM64}
        Goto ok
      ${EndIf}
    !endif
    IfSilent +3
      MessageBox MB_OK "${TERMIZARD_ARCH_NOT_SUPPORTED}"
      Quit
    SetErrorLevel 65
    Abort
  ${Else}
    IfSilent +3
      MessageBox MB_OK "${TERMIZARD_WIN10_REQUIRED}"
      Quit
    SetErrorLevel 64
    Abort
  ${EndIf}

  ok:
!macroend

!macro termizard.files
  !ifdef SUPPORTS_AMD64
    ${If} ${IsNativeAMD64}
      File "/oname=${PRODUCT_EXECUTABLE}" "${ARG_AMD64_BINARY}"
    ${EndIf}
  !endif
  !ifdef SUPPORTS_ARM64
    ${If} ${IsNativeARM64}
      File "/oname=${PRODUCT_EXECUTABLE}" "${ARG_ARM64_BINARY}"
    ${EndIf}
  !endif
!macroend

!macro termizard.setShellContext
  ${If} "${REQUEST_EXECUTION_LEVEL}" == "admin"
    SetShellVarContext all
  ${Else}
    SetShellVarContext current
  ${EndIf}
!macroend

!macro termizard.writeUninstaller
  WriteUninstaller "$INSTDIR\uninstall.exe"
  SetRegView 64
  !if "${INSTALL_SCOPE}" == "user"
    WriteRegStr HKCU "${UNINST_KEY}" "Publisher"          "${INFO_COMPANYNAME}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayName"        "${INFO_PRODUCTNAME}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion"     "${INFO_PRODUCTVERSION}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon"        "$INSTDIR\${PRODUCT_EXECUTABLE}"
    WriteRegStr HKCU "${UNINST_KEY}" "UninstallString"    "$\"$INSTDIR\uninstall.exe$\""
    WriteRegStr HKCU "${UNINST_KEY}" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"
    ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
    IntFmt $0 "0x%08X" $0
    WriteRegDWORD HKCU "${UNINST_KEY}" "EstimatedSize" "$0"
  !else
    WriteRegStr HKLM "${UNINST_KEY}" "Publisher"          "${INFO_COMPANYNAME}"
    WriteRegStr HKLM "${UNINST_KEY}" "DisplayName"        "${INFO_PRODUCTNAME}"
    WriteRegStr HKLM "${UNINST_KEY}" "DisplayVersion"     "${INFO_PRODUCTVERSION}"
    WriteRegStr HKLM "${UNINST_KEY}" "DisplayIcon"        "$INSTDIR\${PRODUCT_EXECUTABLE}"
    WriteRegStr HKLM "${UNINST_KEY}" "UninstallString"    "$\"$INSTDIR\uninstall.exe$\""
    WriteRegStr HKLM "${UNINST_KEY}" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"
    ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
    IntFmt $0 "0x%08X" $0
    WriteRegDWORD HKLM "${UNINST_KEY}" "EstimatedSize" "$0"
  !endif
!macroend

!macro termizard.deleteUninstaller
  Delete "$INSTDIR\uninstall.exe"
  SetRegView 64
  !if "${INSTALL_SCOPE}" == "user"
    DeleteRegKey HKCU "${UNINST_KEY}"
  !else
    DeleteRegKey HKLM "${UNINST_KEY}"
  !endif
!macroend
