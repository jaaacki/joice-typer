import { useEffect, useState, type ReactNode } from "react";
import {
  BridgeRequestError,
  cancelHotkeyCapture,
  canPostNativeMessage,
  confirmHotkeyCapture,
  deleteModel,
  downloadModel,
  fetchConfig,
  fetchDevices,
  fetchModel,
  fetchOptions,
  fetchPermissions,
  fetchRuntime,
  openPermissionSettings,
  saveConfig,
  refreshDevices,
  startHotkeyCapture,
  subscribeConfigSaved,
  subscribeDevicesChanged,
  subscribeHotkeyCaptureChanged,
  subscribeModelDownloadProgress,
  subscribeModelChanged,
  subscribePermissionsChanged,
  subscribeRuntimeStateChanged,
  type AppStateSnapshot,
  type ConfigSnapshot,
  type DeviceSnapshot,
  type HotkeyCaptureSnapshot,
  type ModelDownloadProgressSnapshot,
  type ModelSnapshot,
  type SettingsOptionsSnapshot,
  type PermissionsSnapshot,
  useModel,
} from "../bridge";

type SettingsScreenProps = {
  config: ConfigSnapshot;
  appState: AppStateSnapshot;
};

type FieldProps = {
  label: string;
  children: ReactNode;
};

function Field({ label, children }: FieldProps) {
  return (
    <label className="settings-field">
      <span className="settings-field__label">{label}</span>
      {children}
    </label>
  );
}

function formatTriggerKeyDisplay(keys: string[]): string {
  const nameMap: Record<string, string> = {
    fn: "Fn",
    shift: "Shift",
    ctrl: "Ctrl",
    option: "Option",
    cmd: "Cmd",
    space: "Space",
    tab: "Tab",
    return: "Return",
    escape: "Escape",
    delete: "Delete",
  };
  return keys.map((key) => nameMap[key] ?? key.toUpperCase()).join(" + ");
}

export function SettingsScreen({ config, appState }: SettingsScreenProps) {
  const [draft, setDraft] = useState<ConfigSnapshot>(config);
  const [currentAppState, setCurrentAppState] = useState<AppStateSnapshot>(appState);
  const [permissions, setPermissions] = useState<PermissionsSnapshot>({
    accessibility: false,
    inputMonitoring: false,
  });
  const [devices, setDevices] = useState<DeviceSnapshot[]>([]);
  const [model, setModel] = useState<ModelSnapshot>({
    size: config.modelSize,
    path: "",
    ready: false,
  });
  const [options, setOptions] = useState<SettingsOptionsSnapshot>({
    models: [],
    languages: [],
    decodeModes: [],
    punctuationModes: [],
  });
  const [status, setStatus] = useState<string>("Ready to save");
  const [saving, setSaving] = useState(false);
  const [confirmDeleteModelSize, setConfirmDeleteModelSize] = useState<string | null>(null);
  const [hotkeyCapture, setHotkeyCapture] = useState<HotkeyCaptureSnapshot | null>(null);

  const saveAvailable = canPostNativeMessage();
  const modelActionSize = draft.modelSize;
  const modelMatchesTarget = model.size === modelActionSize;
  const triggerKeyDisplay = hotkeyCapture?.display || formatTriggerKeyDisplay(draft.triggerKey);

  function update<K extends keyof ConfigSnapshot>(key: K, value: ConfigSnapshot[K]) {
    setDraft((current) => ({
      ...current,
      [key]: value,
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

  useEffect(() => subscribeRuntimeStateChanged((nextState) => {
    setCurrentAppState((current) => ({
      state: nextState.state,
      version: nextState.version || current.version,
    }));
  }), []);

  useEffect(() => subscribePermissionsChanged((nextPermissions) => {
    setPermissions(nextPermissions);
  }), []);

  useEffect(() => subscribeModelChanged((nextModel) => {
    setModel(nextModel);
  }), []);

  useEffect(() => subscribeDevicesChanged((nextDevices) => {
    setDevices(nextDevices);
  }), []);

  useEffect(() => subscribeHotkeyCaptureChanged((snapshot) => {
    setHotkeyCapture(snapshot);
  }), []);

  useEffect(() => subscribeModelDownloadProgress((progress: ModelDownloadProgressSnapshot) => {
    const pct = Math.round(progress.progress * 100);
    setStatus(`Downloading ${progress.size} model: ${pct}%`);
  }), []);

  useEffect(() => subscribeConfigSaved((savedConfig) => {
    setDraft(savedConfig);
    setStatus("Config saved. JoiceTyper is reloading the runtime.");
  }), []);

  useEffect(() => {
    setConfirmDeleteModelSize(null);
  }, [modelActionSize]);

  useEffect(() => {
    let cancelled = false;

    async function refreshSettingsContext() {
      try {
        const [nextConfig, nextPermissions, nextDevices, nextModel, nextRuntime, nextOptions] = await Promise.all([
          fetchConfig(),
          fetchPermissions(),
          fetchDevices(),
          fetchModel(),
          fetchRuntime(),
          fetchOptions(),
        ]);
        if (cancelled) {
          return;
        }
        setDraft(nextConfig);
        setPermissions(nextPermissions);
        setDevices(nextDevices);
        setModel(nextModel);
        setCurrentAppState(nextRuntime);
        setOptions(nextOptions);
      } catch (error) {
        if (!cancelled) {
          setStatus(describeBridgeError(error, "Failed to refresh settings context"));
        }
      }
    }

    void refreshSettingsContext();
    return () => {
      cancelled = true;
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
      setStatus(`Starting ${size} model download...`);
      await downloadModel(size);
      setStatus(`Downloaded ${size} model.`);
    } catch (error) {
      setStatus(describeBridgeError(error, `Failed to download ${size} model`));
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

  return (
    <main className="app-shell">
      <section className="settings-screen">
        <header className="settings-screen__header">
          <div>
            <p className="eyebrow">Embedded Settings</p>
            <h1>JoiceTyper Preferences</h1>
          </div>
          <div className="settings-screen__meta">
            <span className="version-chip">{currentAppState.version}</span>
            <div className="status-pill">
              <span className="status-pill__label">Runtime</span>
              <strong>{currentAppState.state}</strong>
            </div>
          </div>
        </header>

        <div className="settings-grid">
          <section className="settings-panel">
            <h2>Capture</h2>
            <Field label="Trigger keys">
              <div className="settings-panel__split">
                <div className="settings-input" aria-live="polite">
                  {triggerKeyDisplay || "No hotkey selected"}
                </div>
                {!hotkeyCapture?.recording ? (
                  <button className="settings-save" type="button" onClick={() => void handleStartHotkeyCapture()}>
                    Change Hotkey
                  </button>
                ) : (
                  <>
                    <button className="settings-save" type="button" onClick={() => void handleCancelHotkeyCapture()}>
                      Cancel
                    </button>
                    <button
                      className="settings-save"
                      type="button"
                      onClick={() => void handleConfirmHotkeyCapture()}
                      disabled={!hotkeyCapture.canConfirm}
                    >
                      Confirm Hotkey
                    </button>
                  </>
                )}
              </div>
            </Field>
            <Field label="Input device">
              {devices.length > 0 ? (
                <div className="settings-panel__split">
                  <select
                    className="settings-input"
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
                  <button className="settings-save" type="button" onClick={() => void handleRefreshDevices()}>
                    Refresh Devices
                  </button>
                </div>
              ) : (
                <input
                  className="settings-input"
                  value={draft.inputDevice}
                  onChange={(event) => update("inputDevice", event.target.value)}
                  placeholder="System default"
                />
              )}
            </Field>
            <Field label="Sample rate">
              <input
                className="settings-input"
                type="number"
                value={draft.sampleRate}
                onChange={(event) => update("sampleRate", Number(event.target.value))}
              />
            </Field>
            <label className="settings-toggle">
              <input
                type="checkbox"
                checked={draft.soundFeedback}
                onChange={(event) => update("soundFeedback", event.target.checked)}
              />
              <span>Sound feedback</span>
            </label>
          </section>

          <section className="settings-panel">
            <h2>Transcription</h2>
            <Field label="Model size">
              <select
                className="settings-input"
                value={draft.modelSize}
                onChange={(event) => update("modelSize", event.target.value)}
              >
                {options.models.map((option) => (
                  <option key={option.code} value={option.code}>
                    {option.name}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Language">
              <select
                className="settings-input"
                value={draft.language}
                onChange={(event) => update("language", event.target.value)}
              >
                {options.languages.map((option) => (
                  <option key={option.code} value={option.code}>
                    {option.name}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Decode mode">
              <select
                className="settings-input"
                value={draft.decodeMode}
                onChange={(event) => update("decodeMode", event.target.value)}
              >
                {options.decodeModes.map((option) => (
                  <option key={option.code} value={option.code}>
                    {option.name}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Punctuation">
              <select
                className="settings-input"
                value={draft.punctuationMode}
                onChange={(event) => update("punctuationMode", event.target.value)}
              >
                {options.punctuationModes.map((option) => (
                  <option key={option.code} value={option.code}>
                    {option.name}
                  </option>
                ))}
              </select>
            </Field>
          </section>

          <section className="settings-panel settings-panel--wide">
            <h2>Vocabulary</h2>
            <textarea
              className="settings-textarea"
              value={draft.vocabulary}
              onChange={(event) => update("vocabulary", event.target.value)}
              placeholder="Comma-separated custom terms"
            />
          </section>

          <section className="settings-panel">
            <h2>Permissions</h2>
            <p className="body">Accessibility: <strong>{permissions.accessibility ? "granted" : "missing"}</strong></p>
            <p className="body">Input Monitoring: <strong>{permissions.inputMonitoring ? "granted" : "missing"}</strong></p>
            <div className="settings-panel__split">
              <button className="settings-save" type="button" onClick={() => void handleOpenPermissionSettings("accessibility", "Accessibility")}>
                Open Accessibility
              </button>
              <button className="settings-save" type="button" onClick={() => void handleOpenPermissionSettings("input_monitoring", "Input Monitoring")}>
                Open Input Monitoring
              </button>
            </div>
          </section>

          <section className="settings-panel">
            <h2>Model Cache</h2>
            <p className="body">Config target: <strong>{modelActionSize}</strong></p>
            <p className="body">Active session model: <strong>{model.size || "unknown"}</strong></p>
            <p className="body">Cached for active model: <strong>{model.ready ? "yes" : "no"}</strong></p>
            <p className="body">Active model path: <strong>{model.path || "unavailable"}</strong></p>
            {!modelMatchesTarget ? (
              <p className="body">Save to keep the config target aligned with the active session model.</p>
            ) : null}
            <div className="settings-panel__split">
              <button className="settings-save" type="button" onClick={() => void handleDownloadModel(modelActionSize)}>
                Download Model
              </button>
              <button className="settings-save" type="button" onClick={() => void handleDeleteModel(modelActionSize)}>
                {confirmDeleteModelSize === modelActionSize ? "Confirm Delete" : "Delete Model"}
              </button>
            </div>
            <div className="settings-panel__split">
              <button className="settings-save" type="button" onClick={() => void handleUseModel(modelActionSize)}>
                Use Model
              </button>
            </div>
          </section>
        </div>

        <footer className="settings-footer">
          <p className="settings-footer__status">{status}</p>
          <button className="settings-save" onClick={() => void handleSave()} disabled={!saveAvailable || saving}>
            Save and Reload
          </button>
        </footer>
      </section>
    </main>
  );
}
