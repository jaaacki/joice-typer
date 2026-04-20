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
Compression=lzma
SolidCompression=yes
ArchitecturesAllowed=x64
ArchitecturesInstallIn64BitMode=x64
WizardStyle=modern
PrivilegesRequired=lowest

[Files]
Source: "{#MyAppSourceDir}\{#MyAppExeName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#MyAppSourceDir}\whisper.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#MyAppSourceDir}\ggml.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#MyAppSourceDir}\ggml-base.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#MyAppSourceDir}\ggml-cpu.dll"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\JoiceTyper"; Filename: "{app}\{#MyAppExeName}"
Name: "{autodesktop}\JoiceTyper"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Additional shortcuts:"

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "Launch JoiceTyper"; Flags: nowait postinstall skipifsilent
