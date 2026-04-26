#ifndef AppVersion
#define AppVersion "0.0.0"
#endif

#ifndef RepoRoot
#define RepoRoot "."
#endif

#ifndef OutputDir
#define OutputDir "."
#endif

#define MyAppName "JoiceTyper"
#define MyAppPublisher "JoiceTyper"
#define MyAppExeName "joicetyper.exe"
#define MyAppSourceDir AddBackslash(RepoRoot) + "build\\windows-amd64"
#define MyAppIcon AddBackslash(RepoRoot) + "assets\\windows\\joicetyper.ico"
#define MyWebView2Bootstrapper AddBackslash(RepoRoot) + "packaging\\windows\\MicrosoftEdgeWebview2Setup.exe"

#ifexist "{#MyWebView2Bootstrapper}"
  #define HasPackagedWebView2Bootstrapper "1"
#else
  #define HasPackagedWebView2Bootstrapper "0"
#endif

[Setup]
AppId={{A9CDAA24-1E98-4A66-8407-12F830792D4D}
AppName={#MyAppName}
AppVersion={#AppVersion}
AppPublisher={#MyAppPublisher}
DefaultDirName={autopf}\JoiceTyper
DefaultGroupName=JoiceTyper
DisableProgramGroupPage=yes
OutputDir={#OutputDir}
OutputBaseFilename=JoiceTyper-{#AppVersion}-setup
SetupIconFile={#MyAppIcon}
Compression=lzma
SolidCompression=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
WizardStyle=modern
PrivilegesRequired=lowest

[Files]
Source: "{#MyAppSourceDir}\{#MyAppExeName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#MyAppIcon}"; DestDir: "{app}"; DestName: "joicetyper.ico"; Flags: ignoreversion
Source: "{#MyAppSourceDir}\whisper.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#MyAppSourceDir}\libwhisper.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#MyAppSourceDir}\ggml.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#MyAppSourceDir}\ggml-base.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#MyAppSourceDir}\ggml-cpu.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#MyAppSourceDir}\ggml-vulkan.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#MyAppSourceDir}\libgcc_s_seh-1.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#MyAppSourceDir}\libstdc++-6.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#MyAppSourceDir}\libgomp-1.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#MyAppSourceDir}\libdl.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#MyAppSourceDir}\libwinpthread-1.dll"; DestDir: "{app}"; Flags: ignoreversion skipifsourcedoesntexist
Source: "{#MyWebView2Bootstrapper}"; DestDir: "{tmp}"; DestName: "MicrosoftEdgeWebview2Setup.exe"; Flags: deleteafterinstall skipifsourcedoesntexist

[Icons]
Name: "{autoprograms}\JoiceTyper"; Filename: "{app}\{#MyAppExeName}"
Name: "{autodesktop}\JoiceTyper"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Additional shortcuts:"

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "Launch JoiceTyper"; Flags: nowait postinstall skipifsilent

[Code]
const
  WebView2ClientGuid = '{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}';

function ValidWebView2Version(const Value: string): Boolean;
begin
  Result := (Value <> '') and (Value <> '0.0.0.0');
end;

function HasWebView2Runtime(): Boolean;
var
  Version: string;
begin
  Result :=
    (RegQueryStringValue(HKLM, 'SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\' + WebView2ClientGuid, 'pv', Version) and ValidWebView2Version(Version)) or
    (RegQueryStringValue(HKCU, 'Software\Microsoft\EdgeUpdate\Clients\' + WebView2ClientGuid, 'pv', Version) and ValidWebView2Version(Version)) or
    (RegQueryStringValue(HKLM, 'SOFTWARE\Microsoft\EdgeUpdate\Clients\' + WebView2ClientGuid, 'pv', Version) and ValidWebView2Version(Version));
end;

function HasWebView2Bootstrapper(): Boolean;
begin
  Result := FileExists(ExpandConstant('{tmp}\MicrosoftEdgeWebview2Setup.exe'));
end;

function NeedsWebView2RuntimeInstall(): Boolean;
begin
  Result := (not HasWebView2Runtime()) and HasWebView2Bootstrapper();
end;

function InstallWebView2Runtime(): Boolean;
var
  ResultCode: Integer;
begin
  if HasWebView2Runtime() then begin
    Result := True;
    exit;
  end;
  if not HasWebView2Bootstrapper() then begin
    Result := False;
    exit;
  end;

  WizardForm.StatusLabel.Caption := 'Installing Microsoft Edge WebView2 Runtime...';
  Result := Exec(ExpandConstant('{tmp}\MicrosoftEdgeWebview2Setup.exe'), '/silent /install', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  if Result and (ResultCode = 0) then
    Result := HasWebView2Runtime()
  else
    Result := False;
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  Result := '';
  if (not HasWebView2Runtime()) and ('{#HasPackagedWebView2Bootstrapper}' <> '1') then
    Result := 'Microsoft Edge WebView2 Runtime is not installed, and this installer does not include MicrosoftEdgeWebview2Setup.exe. Bundle the bootstrapper with the installer or install WebView2 before launching JoiceTyper.';
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if (CurStep = ssPostInstall) and NeedsWebView2RuntimeInstall() and (not InstallWebView2Runtime()) then
    RaiseException('WebView2 Runtime installation did not complete successfully.');
end;
