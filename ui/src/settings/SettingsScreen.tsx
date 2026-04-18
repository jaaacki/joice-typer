import type { AppStateSnapshot, ConfigSnapshot } from "../bridge";

type SettingsScreenProps = {
  config: ConfigSnapshot;
  appState: AppStateSnapshot;
};

type FieldProps = {
  label: string;
  value: string;
};

function Field({ label, value }: FieldProps) {
  return (
    <div className="settings-field">
      <span className="settings-field__label">{label}</span>
      <strong className="settings-field__value">{value}</strong>
    </div>
  );
}

export function SettingsScreen({ config, appState }: SettingsScreenProps) {
  const triggerDisplay = config.triggerKey.length > 0 ? config.triggerKey.join(" + ") : "Not configured";
  const vocabularyDisplay = config.vocabulary.trim() === "" ? "No custom vocabulary" : config.vocabulary;

  return (
    <main className="app-shell">
      <section className="settings-screen">
        <header className="settings-screen__header">
          <div>
            <p className="eyebrow">Embedded Settings Preview</p>
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
            <Field label="Trigger" value={triggerDisplay} />
            <Field label="Input device" value={config.inputDevice || "System default"} />
            <Field label="Sample rate" value={`${config.sampleRate} Hz`} />
            <Field label="Sound feedback" value={config.soundFeedback ? "Enabled" : "Disabled"} />
          </section>

          <section className="settings-panel">
            <h2>Transcription</h2>
            <Field label="Model size" value={config.modelSize} />
            <Field label="Language" value={config.language} />
            <Field label="Decode mode" value={config.decodeMode} />
            <Field label="Punctuation" value={config.punctuationMode} />
          </section>

          <section className="settings-panel settings-panel--wide">
            <div className="settings-panel__split">
              <h2>Vocabulary</h2>
              <span className="version-chip">{appState.version}</span>
            </div>
            <p className="vocabulary-block">{vocabularyDisplay}</p>
          </section>
        </div>
      </section>
    </main>
  );
}
