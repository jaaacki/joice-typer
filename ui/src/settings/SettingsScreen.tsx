import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  BridgeRequestError,
  type BootstrapPayload,
  cancelHotkeyCapture,
  canPostNativeMessage,
  confirmHotkeyCapture,
  deleteModel,
  downloadModel,
  openPermissionSettings,
  refreshDevices,
  saveConfig,
  startHotkeyCapture,
  subscribeConfigSaved,
  subscribeDevicesChanged,
  subscribeHotkeyCaptureChanged,
  subscribeModelDownloadProgress,
  subscribeModelChanged,
  subscribePermissionsChanged,
  subscribeRuntimeStateChanged,
  useModel,
  type AppStateSnapshot,
  type ConfigSnapshot,
  type DeviceSnapshot,
  type HotkeyCaptureSnapshot,
  type ModelDownloadProgressSnapshot,
  type ModelSnapshot,
  type PermissionsSnapshot,
  type SettingsOptionsSnapshot,
} from "../bridge";
import AboutPane from "./panes/AboutPane";
import PermissionsPane from "./panes/PermissionsPane";
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
  { id: "transcription", label: "Transcription", sublabel: "Model & language", icon: <WaveformIcon /> },
  { id: "vocabulary", label: "Vocabulary", sublabel: "Terms & fixes", icon: <BookIcon /> },
  { id: "permissions", label: "Permissions", sublabel: "System access", icon: <ShieldIcon /> },
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
    <svg width="22" height="22" viewBox="0 0 32 32" fill="none" aria-hidden="true">
      <path
        d="M5 9a4 4 0 0 1 4-4h14a4 4 0 0 1 4 4v9a4 4 0 0 1-4 4h-7.5L10 27.5V22H9a4 4 0 0 1-4-4V9z"
        className="settings-logo__bubble"
        strokeWidth="2"
        strokeLinejoin="round"
      />
      <path
        d="M18 10v6.2a2.8 2.8 0 0 1-5.6 0"
        className="settings-logo__mark"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
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

function MicMeter() {
  return (
    <div className="mic-meter" aria-hidden="true">
      {Array.from({ length: 16 }, (_, index) => (
        <span key={index} className="mic-meter__bar" style={{ animationDelay: `${index * 70}ms` }} />
      ))}
    </div>
  );
}

export function SettingsScreen({ bootstrap }: SettingsScreenProps) {
  const initialConfig = sanitizeConfigSnapshot(bootstrap.config);
  const [activePane, setActivePane] = useState<PaneId>("capture");
  const [draft, setDraft] = useState<ConfigSnapshot>(initialConfig);
  const [currentAppState, setCurrentAppState] = useState<AppStateSnapshot>(bootstrap.appState);
  const [permissions, setPermissions] = useState<PermissionsSnapshot>(bootstrap.permissions);
  const [devices, setDevices] = useState<DeviceSnapshot[]>([]);
  const [model, setModel] = useState<ModelSnapshot>(bootstrap.model);
  const [options, setOptions] = useState<SettingsOptionsSnapshot>(bootstrap.options);
  const [status, setStatus] = useState<string>("Ready to save");
  const [saving, setSaving] = useState(false);
  const [confirmDeleteModelSize, setConfirmDeleteModelSize] = useState<string | null>(null);
  const [hotkeyCapture, setHotkeyCapture] = useState<HotkeyCaptureSnapshot | null>(null);
  const [downloadProgress, setDownloadProgress] = useState<ModelDownloadProgressSnapshot | null>(null);

  const modelOptionsByCode = useMemo(
    () => new Map(options.models.map((option) => [option.code, option])),
    [options.models],
  );
  const saveAvailable = canPostNativeMessage();
  const modelActionSize = draft.modelSize;
  const modelMatchesTarget = model.size === modelActionSize;
  const triggerKeyDisplay = hotkeyCapture?.display || formatTriggerKeyDisplay(draft.triggerKey);
  const runtimeStatus = currentAppState.state || "Unknown";
  const selectedPane = NAV_ITEMS.find((item) => item.id === activePane) ?? NAV_ITEMS[0];
  const selectedModelName = modelOptionsByCode.get(modelActionSize)?.name ?? modelActionSize;
  const activeModelName = modelOptionsByCode.get(model.size || modelActionSize)?.name ?? (model.size || modelActionSize);

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

  function update<K extends keyof ConfigSnapshot>(key: K, value: ConfigSnapshot[K]) {
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
  useEffect(
    () =>
      subscribeModelDownloadProgress((progress: ModelDownloadProgressSnapshot) => {
        const pct = Math.round(progress.progress * 100);
        setDownloadProgress(progress);
        setStatus(`Downloading ${progress.size} model: ${pct}%`);
      }),
    [],
  );
  useEffect(
    () =>
      subscribeConfigSaved((savedConfig) => {
        setDraft(sanitizeConfigSnapshot(savedConfig));
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

  async function handleSave() {
    setSaving(true);
    try {
      setStatus("Save request sent. Waiting for native confirmation.");
      await saveConfig(draft);
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

  async function handleDownloadModel(size: string) {
    try {
      setConfirmDeleteModelSize(null);
      setDownloadProgress({ size, progress: 0, bytesDownloaded: 0, bytesTotal: 0 });
      setStatus(`Starting ${size} model download...`);
      await downloadModel(size);
      setDownloadProgress(null);
      setModelInstalled(size, true);
      setStatus(`Downloaded ${size} model.`);
    } catch (error) {
      setStatus(describeBridgeError(error, `Failed to download ${size} model`));
      setDownloadProgress(null);
    }
  }

  async function handleDeleteModel(size: string) {
    if (confirmDeleteModelSize !== size) {
      setConfirmDeleteModelSize(size);
      setStatus(`Confirm deleting ${size} model.`);
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
              <div className="hotkey-capture">
                <div className={`hotkey-capture__keys${hotkeyCapture?.recording ? " is-recording" : ""}`} aria-live="polite">
                  {(hotkeyCapture?.recording ? triggerKeyDisplay.split(" + ") : draft.triggerKey.map((key) => formatTriggerKeyDisplay([key]))).map((part) => (
                    <span key={part} className="key-cap">
                      {part}
                    </span>
                  ))}
                </div>
                <div className="button-row button-row--wrap">
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
                      value={draft.inputDevice}
                      onChange={(event) => update("inputDevice", event.target.value)}
                    >
                      <option value="">System default</option>
                      {devices.map((device) => (
                        <option key={device.name} value={device.name}>
                          {device.name}
                          {device.isDefault ? " (Default)" : ""}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <input
                      className="ui-input"
                      value={draft.inputDevice}
                      onChange={(event) => update("inputDevice", event.target.value)}
                      placeholder="System default"
                    />
                  )}
                  <button className="ui-button ui-button--ghost" type="button" onClick={() => void handleRefreshDevices()}>
                    Refresh Devices
                  </button>
                </div>
              </Field>

              <div className="mic-preview">
                <span className="mic-preview__icon">
                  <MicIcon />
                </span>
                <MicMeter />
                <span className="mic-preview__label">{devices.length > 0 ? "Live input ready" : "Awaiting device list"}</span>
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
            onLanguageChange={(value) => update("language", value)}
            onPunctuationModeChange={(value) => update("punctuationMode", value)}
            onUseModel={handleUseModel}
          />
        );

      case "vocabulary":
        return <VocabularyPane draft={draft} onVocabularyChange={(value) => update("vocabulary", value)} />;

      case "permissions":
        return <PermissionsPane permissions={permissions} onOpenPermissionSettings={handleOpenPermissionSettings} />;

      case "about":
        return (
          <AboutPane
            activeModelName={activeModelName}
            currentAppState={currentAppState}
            runtimeStatus={runtimeStatus}
            saveAvailable={saveAvailable}
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
              {NAV_ITEMS.map((item) => (
                <SidebarButton key={item.id} item={item} active={item.id === activePane} onClick={() => setActivePane(item.id)} />
              ))}
            </nav>

            <div className="settings-sidebar__footer">
              <div className="settings-sidebar__hotkey">
                <span>Idle · Press</span>
                {draft.triggerKey.map((key) => (
                  <span key={key} className="key-cap key-cap--accent">
                    {formatTriggerKeyDisplay([key])}
                  </span>
                ))}
              </div>
              <div className="settings-sidebar__model">Selected model · {selectedModelName}</div>
            </div>
          </aside>

          <div className="settings-content">
            <header className="settings-content__header">
              <p className="eyebrow">{selectedPane.sublabel}</p>
              <h1>{selectedPane.label}</h1>
            </header>

            <div className="settings-content__body">{renderPane()}</div>

            <footer className="settings-footer">
              <p className="settings-footer__status">{status}</p>
              <button className="ui-button ui-button--primary ui-button--large" onClick={() => void handleSave()} disabled={!saveAvailable || saving}>
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
