import { useEffect, useMemo, useState, type ReactNode } from "react";
import logoLightRaw from "../assets/joicetyper-logo.svg?raw";
import logoDarkRaw from "../assets/joicetyper-logo-dark.svg?raw";
const logoLight = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(logoLightRaw)}`;
const logoDark = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(logoDarkRaw)}`;
import {
  BridgeRequestError,
  copyVisibleLogTail,
  type BootstrapPayload,
  cancelHotkeyCapture,
  canPostNativeMessage,
  checkForUpdates,
  confirmHotkeyCapture,
  copyFullLog,
  deleteModel,
  downloadModel,
  fetchUpdater,
  fetchPermissions,
  fetchOptions,
  fetchLogs,
  openPermissionSettings,
  refreshDevices,
  saveConfig,
  fetchLoginItem,
  setLoginItem as setLoginItemApi,
  fetchInputVolume,
  setInputVolume as setInputVolumeApi,
  setAudioInputMonitor,
  stopAudioInputMonitor,
  startHotkeyCapture,
  subscribeConfigSaved,
  subscribeDevicesChanged,
  subscribeHotkeyCaptureChanged,
  subscribeInputLevelChanged,
  subscribeLogsUpdated,
  subscribeModelDownloadCompleted,
  subscribeModelDownloadFailed,
  subscribeModelDownloadProgress,
  subscribeModelChanged,
  subscribePermissionsChanged,
  subscribeRuntimeStateChanged,
  useModel,
  type AppStateSnapshot,
  type ConfigSnapshot,
  type DeviceSnapshot,
  type HotkeyCaptureSnapshot,
  type InputLevelSnapshot,
  type MachineInfoSnapshot,
  type ModelDownloadProgressSnapshot,
  type ModelDownloadFailedSnapshot,
  type ModelSnapshot,
  type LoginItemSnapshot,
  type InputVolumeSnapshot,
  type PermissionsSnapshot,
  type SettingsOptionsSnapshot,
  type UpdaterSnapshot,
} from "../bridge";
import AboutPane from "./panes/AboutPane";
import PermissionsPane from "./panes/PermissionsPane";
import LogsPane from "./panes/LogsPane";
import TranscriptionPane from "./panes/TranscriptionPane";
import VocabularyPane from "./panes/VocabularyPane";
import { Field, Panel, StatusBadge, formatTriggerKeyDisplay, runtimeTone, type PaneId } from "./shared";

type SettingsScreenProps = {
  bootstrap: BootstrapPayload;
};

type NavItem = {
  id: PaneId;
  label: string;
  sublabel: string;
  icon: ReactNode;
};

const NAV_ITEMS: NavItem[] = [
  { id: "capture", label: "Capture", sublabel: "Hotkey & mic", icon: <MicIcon /> },
  { id: "transcription", label: "Mode", sublabel: "Model & language", icon: <WaveformIcon /> },
  { id: "vocabulary", label: "Vocabulary", sublabel: "Terms & fixes", icon: <BookIcon /> },
  { id: "permissions", label: "Permissions", sublabel: "System access", icon: <ShieldIcon /> },
  { id: "logs", label: "Logs", sublabel: "Live tail", icon: <ConsoleIcon /> },
  { id: "about", label: "About", sublabel: "Version & runtime", icon: <InfoIcon /> },
];

function SidebarButton({
  item,
  active,
  onClick,
}: {
  item: NavItem;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button className={`settings-sidebar__item${active ? " is-active" : ""}`} type="button" onClick={onClick}>
      <span className="settings-sidebar__item-icon">{item.icon}</span>
      <span className="settings-sidebar__item-copy">
        <span className="settings-sidebar__item-label">{item.label}</span>
        <span className="settings-sidebar__item-sublabel">{item.sublabel}</span>
      </span>
    </button>
  );
}

function JoiceLogo() {
  return (
    <picture>
      <source srcSet={logoDark} media="(prefers-color-scheme: dark)" />
      <img src={logoLight} alt="JoiceTyper" width="28" height="28" style={{ display: "block", objectFit: "contain" }} />
    </picture>
  );
}

function sanitizeVocabulary(value: string): string {
  return value.replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/g, "");
}

function sanitizeConfigSnapshot(snapshot: ConfigSnapshot): ConfigSnapshot {
  return {
    ...snapshot,
    vocabulary: sanitizeVocabulary(snapshot.vocabulary),
  };
}

function formatTransferSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return "0 B";
  }
  if (bytes >= 1_000_000_000) {
    return `${(bytes / 1_000_000_000).toFixed(1)} GB`;
  }
  if (bytes >= 1_000_000) {
    return `${(bytes / 1_000_000).toFixed(1)} MB`;
  }
  if (bytes >= 1_000) {
    return `${(bytes / 1_000).toFixed(1)} KB`;
  }
  return `${Math.round(bytes)} B`;
}

function micQualityTone(quality: string): "warn" | "ok" | "idle" {
  switch (quality) {
    case "good":
      return "ok";
    case "acceptable":
      return "warn";
    default:
      return "idle";
  }
}

function MicLevelMeter({ level }: { level: number }) {
  const clamped = Math.max(0, Math.min(1, level));
  return (
    <div className="mic-level" aria-hidden="true">
      <div className="mic-level__bar">
        <span className="mic-level__fill" style={{ width: `${Math.round(clamped * 100)}%` }} />
      </div>
    </div>
  );
}

function effectiveInputDevice(config: ConfigSnapshot, devices: DeviceSnapshot[]): string {
  if (config.inputDevice !== "") {
    return config.inputDevice;
  }
  return devices.find((device) => device.isDefault)?.id ?? "";
}

function effectiveInputDeviceName(config: ConfigSnapshot, devices: DeviceSnapshot[]): string {
  if (config.inputDeviceName !== "") {
    return config.inputDeviceName;
  }
  return devices.find((device) => device.isDefault)?.name ?? "";
}

export function SettingsScreen({ bootstrap }: SettingsScreenProps) {
  const initialConfig = sanitizeConfigSnapshot(bootstrap.config);
  const [activePane, setActivePane] = useState<PaneId>("capture");
  const [draft, setDraft] = useState<ConfigSnapshot>(initialConfig);
  const [savedConfig, setSavedConfig] = useState<ConfigSnapshot>(initialConfig);
  const [currentAppState, setCurrentAppState] = useState<AppStateSnapshot>(bootstrap.appState);
  const [permissions, setPermissions] = useState<PermissionsSnapshot>(bootstrap.permissions);
  const [devices, setDevices] = useState<DeviceSnapshot[]>([]);
  const [model, setModel] = useState<ModelSnapshot>(bootstrap.model);
  const [machineInfo] = useState<MachineInfoSnapshot>(bootstrap.machineInfo);
  const [options, setOptions] = useState<SettingsOptionsSnapshot>(bootstrap.options);
  const [updater, setUpdater] = useState<UpdaterSnapshot>({
    enabled: false,
    supportsManualCheck: false,
    feedURL: "",
    channel: "",
  });
  const [status, setStatus] = useState<string>("");
  const [saving, setSaving] = useState(false);
  const [checkingForUpdates, setCheckingForUpdates] = useState(false);
  const [confirmDeleteModelSize, setConfirmDeleteModelSize] = useState<string | null>(null);
  const [hotkeyCapture, setHotkeyCapture] = useState<HotkeyCaptureSnapshot | null>(null);
  const [downloadProgress, setDownloadProgress] = useState<ModelDownloadProgressSnapshot | null>(null);
  const [inputLevel, setInputLevel] = useState<InputLevelSnapshot>({ level: 0, quality: "poor" });
  const [micTestActive, setMicTestActive] = useState(false);
  const [loginItem, setLoginItem] = useState<LoginItemSnapshot>({ enabled: false });
  const [inputVolume, setInputVolume] = useState<InputVolumeSnapshot>({ volume: 0, supported: false });

  useEffect(() => {
    let cancelled = false;
    void fetchLoginItem().then((snapshot) => {
      if (!cancelled) setLoginItem(snapshot);
    }).catch(() => {});
    return () => { cancelled = true; };
  }, []);

  const modelOptionsByCode = useMemo(
    () => new Map(options.models.map((option) => [option.code, option])),
    [options.models],
  );
  const saveAvailable = canPostNativeMessage();
  const modelActionSize = draft.modelSize;
  const modelMatchesTarget = model.size === modelActionSize;
  const triggerKeyDisplay = hotkeyCapture?.display || formatTriggerKeyDisplay(draft.triggerKey);
  const supportedHotkeyModifiers = useMemo(() => new Set(options.hotkey.modifiers), [options.hotkey.modifiers]);
  const supportedHotkeyKeys = useMemo(() => new Set(options.hotkey.keys), [options.hotkey.keys]);
  const unsupportedTriggerKeys = useMemo(
    () =>
      draft.triggerKey.filter((key) => {
        if (supportedHotkeyModifiers.has(key)) {
          return false;
        }
        return !supportedHotkeyKeys.has(key);
      }),
    [draft.triggerKey, supportedHotkeyKeys, supportedHotkeyModifiers],
  );
  const runtimeStatus = currentAppState.state || "Unknown";
  const permissionsPaneVisible =
    options.permissions.accessibility.required ||
    options.permissions.accessibility.actionable ||
    options.permissions.inputMonitoring.required ||
    options.permissions.inputMonitoring.actionable;
  const navItems = permissionsPaneVisible ? NAV_ITEMS : NAV_ITEMS.filter((item) => item.id !== "permissions");
  const selectedPane = navItems.find((item) => item.id === activePane) ?? navItems[0];
  const selectedModelName = modelOptionsByCode.get(modelActionSize)?.name ?? modelActionSize;
  const activeModelName = modelOptionsByCode.get(model.size || modelActionSize)?.name ?? (model.size || modelActionSize);
  const hasUnsavedChanges = useMemo(
    () => JSON.stringify(draft) !== JSON.stringify(savedConfig),
    [draft, savedConfig],
  );
  const configError: string | null = useMemo(() => {
    const selectedModel = modelOptionsByCode.get(draft.modelSize);
    if (draft.outputMode === "translation" && selectedModel?.englishOnly === true) {
      const hasInstalledMultilingual = options.models.some((m) => !m.englishOnly && m.installed);
      return hasInstalledMultilingual
        ? "Translation requires a multilingual model — select one from the Transcription pane."
        : "Translation requires a multilingual model — download one from the Transcription pane.";
    }
    if (draft.outputMode === "transcription" && draft.language === "en" && selectedModel?.englishOnly === false) {
      const hasInstalledEnOnly = options.models.some((m) => m.englishOnly && m.installed);
      return hasInstalledEnOnly
        ? "English transcription requires an English-only model — select one below."
        : "English transcription requires an English-only model — download one below.";
    }
    return null;
  }, [draft.outputMode, draft.language, draft.modelSize, modelOptionsByCode, options.models]);
  const footerStatus = downloadProgress
    ? `Downloading ${downloadProgress.size} model... ${Math.round(downloadProgress.progress * 100)}%${downloadProgress.bytesTotal > 0 ? ` · ${formatTransferSize(downloadProgress.bytesDownloaded)} / ${formatTransferSize(downloadProgress.bytesTotal)}` : ""}`
    : configError
      ? configError
      : status !== ""
        ? status
        : hasUnsavedChanges
          ? "Unsaved changes."
          : "No unsaved changes.";

  const selectedModelStatus = useMemo(() => {
    if (downloadProgress?.size === modelActionSize) {
      return {
        tone: "warn" as const,
        label: `Downloading ${Math.round(downloadProgress.progress * 100)}%`,
      };
    }
    if (model.size === modelActionSize && model.ready) {
      return {
        tone: "ok" as const,
        label: "Loaded for the current session",
      };
    }
    if (modelActionSize === draft.modelSize) {
      return {
        tone: "idle" as const,
        label: modelMatchesTarget ? "Selected in config" : "Selected, save to keep it",
      };
    }
    return {
      tone: "idle" as const,
      label: "Available",
    };
  }, [downloadProgress, draft.modelSize, model.ready, model.size, modelActionSize, modelMatchesTarget]);

  const resolvedInputDevice = useMemo(() => effectiveInputDevice(draft, devices), [draft, devices]);
  const resolvedInputDeviceName = useMemo(() => effectiveInputDeviceName(draft, devices), [draft, devices]);

  useEffect(() => {
    let cancelled = false;
    void fetchInputVolume(resolvedInputDeviceName).then((snapshot) => {
      if (!cancelled) setInputVolume(snapshot);
    }).catch(() => {
      if (!cancelled) setInputVolume({ volume: 0, supported: false });
    });
    return () => { cancelled = true; };
  }, [resolvedInputDeviceName]);

  function update<K extends keyof ConfigSnapshot>(key: K, value: ConfigSnapshot[K]) {
    setStatus("");
    setDraft((current) => ({
      ...current,
      [key]: key === "vocabulary" ? sanitizeVocabulary(String(value)) : value,
    }));
  }

  function setModelInstalled(size: string, installed: boolean) {
    setOptions((current) => ({
      ...current,
      models: current.models.map((option) => (option.code === size ? { ...option, installed } : option)),
    }));
  }

  function describeBridgeError(error: unknown, fallback: string): string {
    if (error instanceof BridgeRequestError) {
      return error.retriable ? `${error.message} Try again.` : error.message;
    }
    if (error instanceof Error && error.message !== "") {
      return error.message;
    }
    return fallback;
  }

  useEffect(
    () =>
      subscribeRuntimeStateChanged((nextState) => {
        setCurrentAppState((current) => ({
          state: nextState.state,
          version: nextState.version || current.version,
        }));
      }),
    [],
  );

  useEffect(() => subscribePermissionsChanged((nextPermissions) => setPermissions(nextPermissions)), []);
  useEffect(() => {
    if (!permissionsPaneVisible) {
      return;
    }

    const accessibilityNeedsAttention = options.permissions.accessibility.required && !permissions.accessibility;
    const inputMonitoringNeedsAttention = options.permissions.inputMonitoring.required && !permissions.inputMonitoring;
    if (!accessibilityNeedsAttention && !inputMonitoringNeedsAttention) {
      return;
    }

    let cancelled = false;

    const refreshLivePermissions = async () => {
      try {
        const nextPermissions = await fetchPermissions();
        if (!cancelled) {
          setPermissions((current) =>
            current.accessibility === nextPermissions.accessibility &&
            current.inputMonitoring === nextPermissions.inputMonitoring
              ? current
              : nextPermissions,
          );
        }
      } catch {
        // Native bridge event delivery remains the primary update path.
      }
    };

    void refreshLivePermissions();
    const intervalId = window.setInterval(() => {
      void refreshLivePermissions();
    }, 1500);
    const onAttention = () => {
      void refreshLivePermissions();
    };
    window.addEventListener("focus", onAttention);
    document.addEventListener("visibilitychange", onAttention);

    return () => {
      cancelled = true;
      window.clearInterval(intervalId);
      window.removeEventListener("focus", onAttention);
      document.removeEventListener("visibilitychange", onAttention);
    };
  }, [
    options.permissions.accessibility.required,
    options.permissions.inputMonitoring.required,
    permissions.accessibility,
    permissions.inputMonitoring,
    permissionsPaneVisible,
  ]);
  useEffect(() => {
    if (!permissionsPaneVisible && activePane === "permissions") {
      setActivePane("capture");
    }
  }, [activePane, permissionsPaneVisible]);
  useEffect(
    () =>
      subscribeModelChanged((nextModel) => {
        setModel(nextModel);
        setDownloadProgress((current) => (current?.size === nextModel.size && nextModel.ready ? null : current));
      }),
    [],
  );
  useEffect(() => subscribeDevicesChanged((nextDevices) => setDevices(nextDevices)), []);
  useEffect(() => subscribeHotkeyCaptureChanged((snapshot) => setHotkeyCapture(snapshot)), []);
  useEffect(() => subscribeInputLevelChanged((snapshot) => setInputLevel(snapshot)), []);
  useEffect(
    () =>
      subscribeModelDownloadProgress((progress: ModelDownloadProgressSnapshot) => {
        setDownloadProgress(progress);
      }),
    [],
  );
  useEffect(
    () =>
      subscribeModelDownloadCompleted(({ size }) => {
        setDownloadProgress((current) => (current?.size === size ? null : current));
        setModelInstalled(size, true);
        setStatus(`Downloaded ${size} model.`);
      }),
    [],
  );
  useEffect(
    () =>
      subscribeModelDownloadFailed((snapshot: ModelDownloadFailedSnapshot) => {
        setDownloadProgress((current) => (current?.size === snapshot.size ? null : current));
        setStatus(snapshot.message || `Failed to download ${snapshot.size} model`);
      }),
    [],
  );
  useEffect(() => {
    if (downloadProgress === null) {
      return;
    }

    let cancelled = false;

    const refreshDownloadState = async () => {
      try {
        const nextOptions = await fetchOptions();
        if (cancelled) {
          return;
        }
        setOptions(nextOptions);
        const downloaded = nextOptions.models.find((option) => option.code === downloadProgress.size)?.installed === true;
        if (!downloaded) {
          return;
        }
        setDownloadProgress((current) => (current?.size === downloadProgress.size ? null : current));
        setStatus(`Downloaded ${downloadProgress.size} model.`);
      } catch {
        // Completion events remain the primary update path.
      }
    };

    void refreshDownloadState();
    const intervalId = window.setInterval(() => {
      void refreshDownloadState();
    }, 1000);

    return () => {
      cancelled = true;
      window.clearInterval(intervalId);
    };
  }, [downloadProgress]);
  useEffect(
    () =>
      subscribeConfigSaved((savedConfig) => {
        const sanitized = sanitizeConfigSnapshot(savedConfig);
        setDraft(sanitized);
        setSavedConfig(sanitized);
        setStatus("Config saved. JoiceTyper is reloading the runtime.");
      }),
    [],
  );

  useEffect(() => {
    setConfirmDeleteModelSize(null);
  }, [modelActionSize]);


  useEffect(() => {
    let cancelled = false;
    const timeoutId = window.setTimeout(() => {
      void refreshDevices()
        .then((nextDevices) => {
          if (!cancelled) {
            setDevices(nextDevices);
          }
        })
        .catch((error) => {
          if (!cancelled) {
            setStatus(describeBridgeError(error, "Failed to refresh input devices"));
          }
        });
    }, 0);

    return () => {
      cancelled = true;
      window.clearTimeout(timeoutId);
    };
  }, []);

  useEffect(() => {
    if (!saveAvailable) {
      return;
    }
    let cancelled = false;
    void fetchUpdater()
      .then((snapshot) => {
        if (!cancelled) {
          setUpdater(snapshot);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setUpdater({
            enabled: false,
            supportsManualCheck: false,
            feedURL: "",
            channel: "",
          });
        }
      });
    return () => {
      cancelled = true;
    };
  }, [saveAvailable]);

  async function handleSave() {
    setSaving(true);
    try {
      setStatus("Save request sent. Waiting for native confirmation.");
      await saveConfig({
        ...draft,
        inputDevice: resolvedInputDevice,
        inputDeviceName: resolvedInputDeviceName,
      });
      setStatus("Saved. JoiceTyper is reloading the runtime.");
    } catch (error) {
      setStatus(describeBridgeError(error, "Failed to save settings"));
    } finally {
      setSaving(false);
    }
  }

  async function handleOpenPermissionSettings(target: "accessibility" | "input_monitoring", label: string) {
    try {
      setStatus(`Opening ${label} settings...`);
      await openPermissionSettings(target);
      setStatus(`Requested ${label} settings.`);
    } catch (error) {
      setStatus(describeBridgeError(error, `Failed to open ${label} settings`));
    }
  }

  async function handleStartHotkeyCapture() {
    try {
      setStatus("Starting hotkey capture...");
      const snapshot = await startHotkeyCapture();
      setHotkeyCapture(snapshot);
      setStatus("Press keys to record the new hotkey.");
    } catch (error) {
      setStatus(describeBridgeError(error, "Failed to start hotkey capture"));
    }
  }

  async function handleCancelHotkeyCapture() {
    try {
      await cancelHotkeyCapture();
      setHotkeyCapture(null);
      setStatus("Hotkey capture cancelled.");
    } catch (error) {
      setStatus(describeBridgeError(error, "Failed to cancel hotkey capture"));
    }
  }

  async function handleConfirmHotkeyCapture() {
    try {
      const snapshot = await confirmHotkeyCapture();
      update("triggerKey", snapshot.triggerKey);
      setHotkeyCapture(null);
      setStatus("Hotkey updated. Save to apply it.");
    } catch (error) {
      setStatus(describeBridgeError(error, "Failed to confirm hotkey capture"));
    }
  }

  async function handleRefreshDevices() {
    try {
      setStatus("Refreshing input devices...");
      const nextDevices = await refreshDevices();
      setDevices(nextDevices);
      setStatus("Input devices refreshed.");
    } catch (error) {
      setStatus(describeBridgeError(error, "Failed to refresh input devices"));
    }
  }

  async function handleStartMicTest() {
    try {
      setStatus("Starting mic test...");
      await setAudioInputMonitor(draft.inputDevice);
      setMicTestActive(true);
      setStatus("Mic test is running.");
    } catch (error) {
      setMicTestActive(false);
      setStatus(describeBridgeError(error, "Failed to start mic test"));
    }
  }

  async function handleStopMicTest() {
    try {
      await stopAudioInputMonitor();
      setMicTestActive(false);
      setInputLevel({ level: 0, quality: "poor" });
      setStatus("Mic test stopped.");
    } catch (error) {
      setStatus(describeBridgeError(error, "Failed to stop mic test"));
    }
  }

  async function handleToggleLoginItem() {
    try {
      const next = await setLoginItemApi(!loginItem.enabled);
      setLoginItem(next);
    } catch (error) {
      setStatus(describeBridgeError(error, "Failed to update login item"));
    }
  }

  async function handleInputVolumeChange(volume: number) {
    setInputVolume((cur) => ({ ...cur, volume }));
    try {
      const next = await setInputVolumeApi(resolvedInputDeviceName, volume);
      setInputVolume(next);
    } catch (error) {
      setStatus(describeBridgeError(error, "Failed to set input volume"));
    }
  }

  async function handleCheckForUpdates() {
    setCheckingForUpdates(true);
    try {
      setStatus("Checking for updates...");
      await checkForUpdates();
      setStatus("Update check started.");
    } catch (error) {
      setStatus(describeBridgeError(error, "Failed to check for updates"));
    } finally {
      setCheckingForUpdates(false);
    }
  }

  async function handleDownloadModel(size: string) {
    try {
      setConfirmDeleteModelSize(null);
      setDownloadProgress({ size, progress: 0, bytesDownloaded: 0, bytesTotal: 0 });
      setStatus("");
      await downloadModel(size);
      setStatus("");
    } catch (error) {
      setStatus(describeBridgeError(error, `Failed to download ${size} model`));
      setDownloadProgress(null);
    }
  }

  async function handleDeleteModel(size: string) {
    if (confirmDeleteModelSize !== size) {
      setConfirmDeleteModelSize(size);
      return;
    }
    try {
      setConfirmDeleteModelSize(null);
      setStatus(`Deleting ${size} model...`);
      await deleteModel(size);
      setModelInstalled(size, false);
      setStatus(`Deleted ${size} model.`);
      if (model.size === size) {
        setModel((current) => ({ ...current, ready: false }));
      }
    } catch (error) {
      setStatus(describeBridgeError(error, `Failed to delete ${size} model`));
    }
  }

  async function handleUseModel(size: string) {
    try {
      setConfirmDeleteModelSize(null);
      setStatus(`Selecting ${size} model...`);
      await useModel(size);
      update("modelSize", size);
      setStatus(`Selected ${size} model for this session. Save to keep it.`);
    } catch (error) {
      setStatus(describeBridgeError(error, `Failed to use ${size} model`));
    }
  }

  function renderPane() {
    switch (activePane) {
      case "capture":
        return (
          <div className="pane-stack">
            <Panel
              eyebrow="Dictation trigger"
              title="Hotkey"
              right={<StatusBadge tone={hotkeyCapture?.recording ? "warn" : "ok"}>{hotkeyCapture?.recording ? "Listening" : "Active"}</StatusBadge>}
            >
              <div className="hotkey-capture hotkey-capture--row">
                <div className={`hotkey-capture__keys${hotkeyCapture?.recording ? " is-recording" : ""}`} aria-live="polite">
                  {(hotkeyCapture?.recording ? triggerKeyDisplay.split(" + ") : draft.triggerKey.map((key) => formatTriggerKeyDisplay([key]))).map((part) => (
                    <span key={part} className="key-cap">
                      {part}
                    </span>
                  ))}
                </div>
                <div className="button-row">
                  {!hotkeyCapture?.recording ? (
                    <button className="ui-button ui-button--secondary" type="button" onClick={() => void handleStartHotkeyCapture()}>
                      Change Hotkey
                    </button>
                  ) : (
                    <>
                      <button className="ui-button ui-button--secondary" type="button" onClick={() => void handleCancelHotkeyCapture()}>
                        Cancel
                      </button>
                      <button
                        className="ui-button ui-button--primary"
                        type="button"
                        onClick={() => void handleConfirmHotkeyCapture()}
                        disabled={!hotkeyCapture.canConfirm}
                      >
                        Confirm Hotkey
                      </button>
                    </>
                  )}
                </div>
              </div>

              {unsupportedTriggerKeys.length > 0 ? (
                <p className="settings-inline-warning">
                  This platform does not support the current hotkey: {unsupportedTriggerKeys.map((key) => formatTriggerKeyDisplay([key])).join(", ")}.
                </p>
              ) : null}

              <div className="settings-inline-toggle">
                <label className="switch">
                  <input
                    type="checkbox"
                    checked={draft.soundFeedback}
                    onChange={(event) => update("soundFeedback", event.target.checked)}
                  />
                  <span className="switch__track" />
                  <span className="switch__copy">
                    <strong>Sound feedback</strong>
                    <small>Play a soft chime when dictation starts and stops.</small>
                  </span>
                </label>
              </div>

              {/* Future template slot: hold-to-talk is always on in the current runtime.
              <div className="settings-inline-toggle">
                <label className="switch">
                  <input type="checkbox" checked={true} readOnly />
                  <span className="switch__track" />
                  <span className="switch__copy">
                    <strong>Hold-to-talk</strong>
                    <small>Release the key to stop dictating.</small>
                  </span>
                </label>
              </div>
              */}
            </Panel>

            <Panel eyebrow="Audio input" title="Microphone">
              <Field label="Input device">
                <div className="input-row">
                  {devices.length > 0 ? (
                    <select
                      className="ui-select"
                      value={resolvedInputDevice}
                      onChange={(event) => {
                        const selected = devices.find((device) => device.id === event.target.value);
                        update("inputDevice", selected?.id ?? "");
                        update("inputDeviceName", selected?.name ?? "");
                        setMicTestActive(false);
                        setInputLevel({ level: 0, quality: "poor" });
                      }}
                    >
                      {devices.map((device) => (
                        <option key={device.id} value={device.id}>
                          {device.name}
                          {device.isDefault ? " (Default)" : ""}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <input
                      className="ui-input"
                      value={resolvedInputDevice}
                      onChange={(event) => update("inputDevice", event.target.value)}
                      placeholder="System default"
                    />
                  )}
                  <button className="ui-button ui-button--ghost" type="button" onClick={() => void handleRefreshDevices()}>
                    Refresh Devices
                  </button>
                </div>
              </Field>

              <Field label="Input volume" hint={inputVolume.supported ? "Adjusts the system input level for this device" : "This device does not support software volume control"}>
                <div className="input-row">
                  <input
                    type="range"
                    min={0}
                    max={1}
                    step={0.01}
                    value={inputVolume.volume}
                    disabled={!inputVolume.supported}
                    onChange={(event) => void handleInputVolumeChange(parseFloat(event.target.value))}
                    style={{ flex: 1 }}
                  />
                  <span style={{ minWidth: "3.5em", textAlign: "right", fontVariantNumeric: "tabular-nums" }}>
                    {inputVolume.supported ? `${Math.round(inputVolume.volume * 100)}%` : "n/a"}
                  </span>
                </div>
              </Field>

              <div className="mic-preview">
                <span className="mic-preview__icon">
                  <MicIcon />
                </span>
                <div className="mic-preview__instruction">
                  Click Start mic test, then speak a short phrase in your normal voice. Click Stop when done.
                </div>
                {micTestActive && (
                  <span className="mic-preview__label">
                    <StatusBadge tone={inputLevel.level === 0 ? "idle" : micQualityTone(inputLevel.quality)}>
                      {inputLevel.level === 0 ? "No detection" : inputLevel.quality === "good" ? "Good" : inputLevel.quality === "acceptable" ? "Acceptable" : "Poor"}
                    </StatusBadge>
                  </span>
                )}
                {!micTestActive ? (
                  <button className="ui-button ui-button--secondary" type="button" onClick={() => void handleStartMicTest()}>
                    Start Mic Test
                  </button>
                ) : (
                  <button className="ui-button ui-button--ghost" type="button" onClick={() => void handleStopMicTest()}>
                    Stop Mic Test
                  </button>
                )}
                <MicLevelMeter level={inputLevel.level} />
              </div>

              {/* Future template slot: advanced capture tuning is intentionally hidden until the backend has hard constraints instead of accepting arbitrary integers.
              <Panel eyebrow="Signal" title="Sample rate">
                <Field label="Sample rate" hint="Whisper expects 16 kHz">
                  <select className="ui-select" value={String(draft.sampleRate)}>
                    <option value="16000">16000 Hz</option>
                  </select>
                </Field>
              </Panel>
              */}
            </Panel>
          </div>
        );

      case "transcription":
        return (
          <TranscriptionPane
            confirmDeleteModelSize={confirmDeleteModelSize}
            downloadProgress={downloadProgress}
            draft={draft}
            model={model}
            modelActionSize={modelActionSize}
            modelMatchesTarget={modelMatchesTarget}
            options={options}
            selectedModelName={selectedModelName}
            activeModelName={activeModelName}
            selectedModelStatus={selectedModelStatus}
            onDecodeModeChange={(value) => update("decodeMode", value)}
            onDeleteModel={handleDeleteModel}
            onDownloadModel={handleDownloadModel}
            onLanguageChange={(newLang) => {
              setStatus("");
              setDraft((cur) => {
                if (cur.outputMode !== "transcription") return { ...cur, language: newLang };
                const curModel = modelOptionsByCode.get(cur.modelSize);
                let newModel = cur.modelSize;
                if (newLang === "en" && !curModel?.englishOnly) {
                  const enEquiv = options.models.find((m) => m.code === cur.modelSize + ".en" && m.installed);
                  const anyEn = options.models.find((m) => m.englishOnly && m.installed);
                  newModel = (enEquiv ?? anyEn)?.code ?? cur.modelSize;
                } else if (newLang !== "en" && curModel?.englishOnly) {
                  const base = options.models.find((m) => m.code === cur.modelSize.replace(".en", "") && m.installed);
                  const anyMulti = options.models.find((m) => !m.englishOnly && m.installed);
                  newModel = (base ?? anyMulti)?.code ?? cur.modelSize;
                }
                return { ...cur, language: newLang, modelSize: newModel };
              });
            }}
            onOutputModeChange={(newMode) => {
              setStatus("");
              setDraft((cur) => {
                const curModel = modelOptionsByCode.get(cur.modelSize);
                let newModel = cur.modelSize;
                if (newMode === "translation" && curModel?.englishOnly) {
                  const base = options.models.find((m) => m.code === cur.modelSize.replace(".en", "") && m.installed);
                  const anyMulti = options.models.find((m) => !m.englishOnly && m.installed);
                  newModel = (base ?? anyMulti)?.code ?? cur.modelSize;
                } else if (newMode === "transcription" && cur.language === "en" && !curModel?.englishOnly) {
                  const enEquiv = options.models.find((m) => m.code === cur.modelSize + ".en" && m.installed);
                  const anyEn = options.models.find((m) => m.englishOnly && m.installed);
                  newModel = (enEquiv ?? anyEn)?.code ?? cur.modelSize;
                }
                return { ...cur, outputMode: newMode, modelSize: newModel };
              });
            }}
            onPunctuationModeChange={(value) => update("punctuationMode", value)}
            onUseModel={handleUseModel}
          />
        );

      case "vocabulary":
        return <VocabularyPane draft={draft} onVocabularyChange={(value) => update("vocabulary", value)} />;

      case "permissions":
        return (
          <PermissionsPane
            options={options.permissions}
            permissions={permissions}
            loginItem={loginItem}
            onOpenPermissionSettings={handleOpenPermissionSettings}
            onToggleLoginItem={handleToggleLoginItem}
          />
        );

      case "logs":
        return <LogsPane copyVisibleLogTail={copyVisibleLogTail} copyFullLog={copyFullLog} fetchLogs={fetchLogs} subscribeLogsUpdated={subscribeLogsUpdated} />;

      case "about":
        return (
          <AboutPane
            activeModelName={activeModelName}
            currentAppState={currentAppState}
            machineInfo={machineInfo}
            runtimeStatus={runtimeStatus}
            saveAvailable={saveAvailable}
            updater={updater}
            checkingForUpdates={checkingForUpdates}
            onCheckForUpdates={() => void handleCheckForUpdates()}
          />
        );
    }
  }

  return (
    <main className="app-shell">
      <section className="settings-screen">
        <header className="settings-screen__header">
          <div className="settings-screen__header-spacer" aria-hidden="true" />
          <div className="settings-screen__meta">
            {savedConfig.outputMode === "translation" ? (
              <StatusBadge tone="warn">Translation active</StatusBadge>
            ) : (
              <StatusBadge tone="warn">Transcription active</StatusBadge>
            )}
            <StatusBadge tone={runtimeTone(runtimeStatus)}>Runtime {runtimeStatus}</StatusBadge>
            <span className="version-chip">{currentAppState.version}</span>
          </div>
        </header>

        <div className="settings-grid">
          <aside className="settings-sidebar">
            <div className="settings-sidebar__brand">
              <div className="settings-sidebar__brand-mark">
                <JoiceLogo />
              </div>
              <div className="settings-sidebar__brand-copy">
                <strong>JoiceTyper</strong>
                <span>Preferences</span>
              </div>
            </div>

            <nav className="settings-sidebar__nav" aria-label="Preferences sections">
            {navItems.map((item) => (
              <SidebarButton key={item.id} item={item} active={item.id === activePane} onClick={() => setActivePane(item.id)} />
            ))}
            </nav>

            <div className="settings-sidebar__footer">
              <div className="settings-sidebar__hotkey">
                <span>Hotkey</span>
                {draft.triggerKey.map((key) => (
                  <span key={key} className="key-cap key-cap--accent">
                    {formatTriggerKeyDisplay([key])}
                  </span>
                ))}
              </div>
            </div>
          </aside>

          <div className="settings-content">
            <header className="settings-content__header">
              <p className="eyebrow">{selectedPane.sublabel}</p>
              <h1>{selectedPane.label}</h1>
            </header>

            <div className="settings-content__body">{renderPane()}</div>

            <footer className="settings-footer">
              <div className="settings-footer__status-wrap">
                <p className="settings-footer__status">{footerStatus}</p>
                {downloadProgress ? (
                  <div className="settings-footer__progress" aria-hidden="true">
                    <span
                      className="settings-footer__progress-fill"
                      style={{ width: `${Math.round(downloadProgress.progress * 100)}%` }}
                    />
                  </div>
                ) : null}
              </div>
              <button className="ui-button ui-button--primary ui-button--large" onClick={() => void handleSave()} disabled={!saveAvailable || saving || configError !== null}>
                Save and Reload
              </button>
            </footer>
          </div>
        </div>
      </section>
    </main>
  );
}

function MicIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <path
        d="M8 10.8a2.8 2.8 0 0 0 2.8-2.8V4.8A2.8 2.8 0 1 0 5.2 4.8V8A2.8 2.8 0 0 0 8 10.8Zm0 0v2.4m-3-5a3 3 0 1 0 6 0"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function WaveformIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <path
        d="M2.2 8h1.3l1.2-3.3 2 7 2-5.1 1.3 2.2h3.8"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function BookIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <path
        d="M4.2 3.2h6a1.8 1.8 0 0 1 1.8 1.8v7.8H5.7a1.5 1.5 0 0 0-1.5 1.5V4.7a1.5 1.5 0 0 1 1.5-1.5Zm0 0v9.6"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function ShieldIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <path
        d="M8 2.2 12 3.8v3.4c0 2.6-1.6 4.8-4 6.6-2.4-1.8-4-4-4-6.6V3.8L8 2.2Zm-1.4 5.7 1.1 1.1 2.1-2.1"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function InfoIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <path
        d="M8 11V7.5m0-2.6h.01M13 8A5 5 0 1 1 3 8a5 5 0 0 1 10 0Z"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function ConsoleIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <path
        d="M3 4.2h10a.8.8 0 0 1 .8.8v6a.8.8 0 0 1-.8.8H3a.8.8 0 0 1-.8-.8V5a.8.8 0 0 1 .8-.8Zm1.1 2.1 1.6 1.7-1.6 1.7m3.2.8h3.3"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.3"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
