; Drift Windows Installer (Inno Setup)
;
; Build locally:
;   iscc installer\drift.iss
;
; Build with a specific version (used by CI):
;   iscc /DDriftVersion=0.2.0 installer\drift.iss
;
; The drift.exe must already be built (with version ldflags) at
; installer\drift.exe before running ISCC.

#ifndef DriftVersion
  #define DriftVersion "0.1.0"
#endif
#define DriftPublisher "Drift"
#define DriftURL "https://github.com/drift/drift"
#define DriftExeName "drift.exe"

[Setup]
AppId={{8F4E6B7A-3D5C-4A2E-9F1B-7C8D6E5A4B3C}
AppName=Drift
AppVersion={#DriftVersion}
AppVerName=Drift {#DriftVersion}
AppPublisher={#DriftPublisher}
AppPublisherURL={#DriftURL}
AppSupportURL={#DriftURL}
AppUpdatesURL={#DriftURL}
DefaultDirName={autopf}\Drift
DefaultGroupName=Drift
DisableProgramGroupPage=yes
OutputDir=.
OutputBaseFilename=drift-setup-{#DriftVersion}
Compression=lzma2
SolidCompression=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
UninstallDisplayIcon={app}\{#DriftExeName}
UninstallDisplayName=Drift {#DriftVersion}
WizardStyle=modern
ChangesEnvironment=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "chinesesimplified"; MessagesFile: "compiler:Languages\ChineseSimplified.isl"
Name: "chinesetraditional"; MessagesFile: "compiler:Languages\ChineseTraditional.isl"

[Tasks]
Name: "addpath"; Description: "Add drift to PATH (recommended)"; GroupDescription: "Environment:"

[Files]
Source: "drift.exe"; DestDir: "{app}"; Flags: ignoreversion

[Run]
Filename: "{app}\{#DriftExeName}"; Parameters: "version"; Description: "Verify installation"; Flags: postinstall skipifsilent runhidden

[UninstallRun]
; No special uninstall steps needed — PATH is handled by [UninstallDelete] + ChangesEnvironment.

[UninstallDelete]
Type: dirifempty; Name: "{app}"

[Code]
// Add drift directory to user PATH on install (if the task is selected).
function NeedsAddPath(Param: string): boolean;
var
  OrigPath: string;
begin
  if not RegQueryStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', OrigPath) then begin
    Result := True;
    exit;
  end;
  Result := Pos(';' + Param + ';', ';' + OrigPath + ';') = 0;
end;

procedure AddPath(Param: string);
var
  OrigPath: string;
begin
  if not RegQueryStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', OrigPath) then begin
    OrigPath := '';
  end;
  if OrigPath <> '' then begin
    OrigPath := OrigPath + ';';
  end;
  RegWriteStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', OrigPath + Param);
end;

procedure RemovePath(Param: string);
var
  OrigPath: string;
  P: Integer;
begin
  if not RegQueryStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', OrigPath) then begin
    exit;
  end;
  P := Pos(';' + Param + ';', ';' + OrigPath + ';');
  if P = 0 then begin
    exit;
  end;
  // Remove the entry from PATH.
  if P = 1 then begin
    // Entry is at the start.
    Delete(OrigPath, 1, Length(Param) + 1);
  end else begin
    Delete(OrigPath, P, Length(Param) + 1);
  end;
  RegWriteStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', OrigPath);
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then begin
    if IsTaskSelected('addpath') then begin
      if NeedsAddPath(ExpandConstant('{app}')) then begin
        AddPath(ExpandConstant('{app}'));
      end;
    end;
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then begin
    RemovePath(ExpandConstant('{app}'));
  end;
end;
