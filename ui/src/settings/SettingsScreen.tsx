import { useState, type ReactNode } from "react";
import { canPostNativeMessage, saveConfig, type AppStateSnapshot, type ConfigSnapshot } from "../bridge";

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

export function SettingsScreen({ config, appState }: SettingsScreenProps) {
  const [draft, setDraft] = useState<ConfigSnapshot>(config);
  const [status, setStatus] = useState<string>("Ready to save");

  const saveAvailable = canPostNativeMessage();

  function update<K extends keyof ConfigSnapshot>(key: K, value: ConfigSnapshot[K]) {
    setDraft((current) => ({
      ...current,
      [key]: value,
    }));
  }

  function handleSave() {
    try {
      saveConfig(draft);
      setStatus("Saved. JoiceTyper is reloading the runtime.");
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Failed to save settings");
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
          <div className="status-pill">
            <span className="status-pill__label">Runtime</span>
            <strong>{appState.state}</strong>
          </div>
        </header>

        <div className="settings-grid">
          <section className="settings-panel">
            <h2>Capture</h2>
            <Field label="Trigger keys">
              <input
                className="settings-input"
                value={draft.triggerKey.join(", ")}
                onChange={(event) =>
                  update(
                    "triggerKey",
                    event.target.value
                      .split(",")
                      .map((value) => value.trim())
                      .filter((value) => value !== ""),
                  )
                }
              />
            </Field>
            <Field label="Input device">
              <input
                className="settings-input"
                value={draft.inputDevice}
                onChange={(event) => update("inputDevice", event.target.value)}
                placeholder="System default"
              />
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
              <input
                className="settings-input"
                value={draft.modelSize}
                onChange={(event) => update("modelSize", event.target.value)}
              />
            </Field>
            <Field label="Language">
              <input
                className="settings-input"
                value={draft.language}
                onChange={(event) => update("language", event.target.value)}
              />
            </Field>
            <Field label="Decode mode">
              <input
                className="settings-input"
                value={draft.decodeMode}
                onChange={(event) => update("decodeMode", event.target.value)}
              />
            </Field>
            <Field label="Punctuation">
              <input
                className="settings-input"
                value={draft.punctuationMode}
                onChange={(event) => update("punctuationMode", event.target.value)}
              />
            </Field>
          </section>

          <section className="settings-panel settings-panel--wide">
            <div className="settings-panel__split">
              <h2>Vocabulary</h2>
              <span className="version-chip">{appState.version}</span>
            </div>
            <textarea
              className="settings-textarea"
              value={draft.vocabulary}
              onChange={(event) => update("vocabulary", event.target.value)}
              placeholder="Comma-separated custom terms"
            />
          </section>
        </div>

        <footer className="settings-footer">
          <p className="settings-footer__status">{status}</p>
          <button className="settings-save" onClick={handleSave} disabled={!saveAvailable}>
            Save and Reload
          </button>
        </footer>
      </section>
    </main>
  );
}
