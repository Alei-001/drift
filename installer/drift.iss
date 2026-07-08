; Inno Setup script for drift Windows installer.
; Docs: https://jrsoftware.org/ishelp/
;
; Built by .github/workflows/release.yml after goreleaser produces the
; windows_amd64 archive. The workflow extracts drift.exe from the zip and
; runs `iscc installer/drift.iss` to produce drift_<version>_windows_amd64_setup.exe.
;
; Build (local, for testing):
;   iscc /DMyAppVersion=0.1.0 /DSourceDir=dist/drift_windows_amd64 installer/drift.iss
;
; Path resolution: Inno Setup resolves relative paths against the .iss file's
; directory (installer/), NOT the working directory. All paths to repo-root
; files therefore use the ..\ prefix.

#ifndef MyAppVersion
  #define MyAppVersion "0.0.0-dev"
#endif

#ifndef SourceDir
  #define SourceDir "..\dist\drift_windows_amd64"
#endif

; OutputBaseFilename uses a fixed name to avoid Inno Setup preprocessor
; issues with version strings (dots are rejected; dashes are parsed as
; arithmetic). The CI workflow renames the output to include the version.

[Setup]
AppName=drift
AppVersion={#MyAppVersion}
AppVerName=drift {#MyAppVersion}
AppPublisher=drift
AppPublisherURL=https://github.com/Alei-001/drift
AppSupportURL=https://github.com/Alei-001/drift/issues
AppUpdatesURL=https://github.com/Alei-001/drift/releases
DefaultDirName={autopf}\drift
DefaultGroupName=drift
DisableProgramGroupPage=yes
OutputDir=..\dist
OutputBaseFilename=drift_windows_setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=admin
; Use the project icon for the installer and the uninstaller.
SetupIconFile=..\assets\icon.ico
UninstallDisplayIcon={app}\drift.exe
LicenseFile=..\LICENSE

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "addpath"; Description: "Add drift to PATH (recommended)"; GroupDescription: "Additional tasks:"

[Files]
Source: "{#SourceDir}\drift.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\README.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\README.zh-CN.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\LICENSE"; DestDir: "{app}"; Flags: ignoreversion

[Run]
; Offer to run drift version after install so the user can verify.
Filename: "{app}\drift.exe"; Parameters: "version"; Description: "Show installed version"; Flags: postinstall nowait skipifsilent runmaximized

[Code]
// PATH management via environment variable APIs (more reliable than setx).
const
  EnvironmentKey = 'SYSTEM\CurrentControlSet\Control\Session Manager\Environment';
  HWND_BROADCAST = $FFFF;
  WM_SETTINGCHANGE = $001A;
  SMTO_ABORTIFHUNG = $0002;

// SendMessageTimeout broadcasts WM_SETTINGCHANGE so new terminals pick up the
// updated PATH without requiring logoff/restart. lParam must be a pointer to
// the string 'Environment'; declaring it as PChar lets Inno Setup pass a
// string literal directly.
function SendMessageTimeout(hWnd: HWND; Msg: UINT; wParam: UINT_PTR;
  lParam: PChar; fuFlags: UINT; uTimeout: UINT;
  out lpdwResult: UINT_PTR): UINT_PTR;
external '[email protected] stdcall';

procedure BroadcastEnvironmentChange;
var
  Dummy: UINT_PTR;
begin
  SendMessageTimeout(HWND_BROADCAST, WM_SETTINGCHANGE, 0, 'Environment',
    SMTO_ABORTIFHUNG, 5000, Dummy);
end;

procedure EnvAddPath(Path: string);
var
  Paths: string;
begin
  if not RegQueryStringValue(HKEY_LOCAL_MACHINE, EnvironmentKey, 'Path', Paths) then
    Paths := '';
  if Pos(';' + Uppercase(Path) + ';', ';' + Uppercase(Paths) + ';') > 0 then
    exit;
  if (Paths <> '') and (Paths[Length(Paths)] <> ';') then
    Paths := Paths + ';';
  Paths := Paths + Path;
  RegWriteStringValue(HKEY_LOCAL_MACHINE, EnvironmentKey, 'Path', Paths);
end;

procedure EnvRemovePath(Path: string);
var
  Paths: string;
  P: Integer;
begin
  if not RegQueryStringValue(HKEY_LOCAL_MACHINE, EnvironmentKey, 'Path', Paths) then
    exit;
  P := Pos(';' + Uppercase(Path) + ';', ';' + Uppercase(Paths) + ';');
  if P = 0 then
    exit;
  // P is 1-based into the padded string (';' + Paths + ';'). The leading ';'
  // shifts positions by 1: when P=1 the path is at the start of Paths; when
  // P>1 the ';' before the path is at P-1 in Paths.
  if P > 1 then
    P := P - 1;
  Delete(Paths, P, Length(Path) + 1);
  RegWriteStringValue(HKEY_LOCAL_MACHINE, EnvironmentKey, 'Path', Paths);
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if (CurStep = ssPostInstall) and IsTaskSelected('addpath') then
  begin
    EnvAddPath(ExpandConstant('{app}'));
    BroadcastEnvironmentChange;
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
  begin
    EnvRemovePath(ExpandConstant('{app}'));
    BroadcastEnvironmentChange;
  end;
end;
