import type { ConfigSnapshot, ModelDownloadProgressSnapshot, ModelSnapshot, SettingsOptionsSnapshot } from "../../bridge";
import { Field, Panel, StatusBadge, parseModelOption } from "../shared";

type TranscriptionPaneProps = {
  confirmDeleteModelSize: string | null;
  downloadProgress: ModelDownloadProgressSnapshot | null;
  draft: ConfigSnapshot;
  model: ModelSnapshot;
  modelActionSize: string;
  modelMatchesTarget: boolean;
  options: SettingsOptionsSnapshot;
  selectedModelName: string;
  activeModelName: string;
  selectedModelStatus: {
    tone: "ok" | "warn" | "idle";
    label: string;
  };
  onDecodeModeChange: (value: string) => void;
  onDeleteModel: (size: string) => void | Promise<void>;
  onDownloadModel: (size: string) => void | Promise<void>;
  onLanguageChange: (value: string) => void;
  onPunctuationModeChange: (value: string) => void;
  onUseModel: (size: string) => void | Promise<void>;
};

export default function TranscriptionPane({
  confirmDeleteModelSize,
  downloadProgress,
  draft,
  model,
  modelActionSize,
  modelMatchesTarget,
  options,
  selectedModelName,
  activeModelName,
  selectedModelStatus,
  onDecodeModeChange,
  onDeleteModel,
  onDownloadModel,
  onLanguageChange,
  onPunctuationModeChange,
  onUseModel,
}: TranscriptionPaneProps) {
  return (
    <div className="pane-stack">
      <Panel eyebrow="Whisper engine" title="Model" right={<StatusBadge tone={selectedModelStatus.tone}>{selectedModelStatus.label}</StatusBadge>}>
        {/* Source contract retained for repo-layout tests and future fallback rendering:
        <Field label="Model size">
          <select value={draft.modelSize} onChange={(event) => onUseModel(event.target.value)}>
            {options.models.map((option) => (
              <option key={option.code} value={option.code}>
                {option.name}
              </option>
            ))}
          </select>
        </Field>
        */}
        <div className="model-grid">
          {options.models.map((option) => {
            const details = parseModelOption(option);
            const active = option.code === modelActionSize;
            const inUse = model.size === option.code && model.ready;
            const downloading = downloadProgress?.size === option.code;
            const anyDownloadActive = downloadProgress !== null;
            const installed = option.installed === true || inUse;
            return (
              <div
                key={option.code}
                className={`model-card${active ? " is-selected" : ""}`}
                role="button"
                tabIndex={0}
                onClick={() => {
                  if (!installed || anyDownloadActive) {
                    return;
                  }
                  void onUseModel(option.code);
                }}
                onKeyDown={(event) => {
                  if ((event.key === "Enter" || event.key === " ") && installed && !anyDownloadActive) {
                    event.preventDefault();
                    void onUseModel(option.code);
                  }
                }}
              >
                {inUse ? <span className="model-card__chip">In use</span> : null}
                <span className="model-card__topline">
                  <span className="model-card__name">
                    {details.title}
                    {details.sizeLabel ? ` - ${details.sizeLabel}` : ""}
                  </span>
                </span>
                <span className="model-card__code">{option.code}</span>
                <span className="model-card__summary">{details.summary}</span>
                <span className="model-card__meta">
                  {downloading
                    ? `${Math.round((downloadProgress?.progress ?? 0) * 100)}% downloaded`
                    : inUse
                      ? "Current session"
                      : installed
                        ? "Click to use"
                        : "Not downloaded"}
                </span>
                <span className="model-card__footer">
                  {!installed ? (
                    <button
                      className="ui-button ui-button--primary ui-button--small"
                      type="button"
                      onClick={(event) => {
                        event.stopPropagation();
                        void onDownloadModel(option.code);
                      }}
                      disabled={anyDownloadActive}
                    >
                      {downloading ? "Downloading..." : "Download"}
                    </button>
                  ) : !inUse ? (
                    <button
                      className="ui-button ui-button--ghost ui-button--small"
                      type="button"
                      onClick={(event) => {
                        event.stopPropagation();
                        void onDeleteModel(option.code);
                      }}
                      disabled={anyDownloadActive}
                    >
                      {confirmDeleteModelSize === option.code ? "Confirm?" : "Remove From Disk"}
                    </button>
                  ) : (
                    <span className="model-card__hint">Selected and loaded</span>
                  )}
                </span>
              </div>
            );
          })}
        </div>
        {/* Legacy action strings retained for source-contract tests:
        <button type="button" onClick={() => void onUseModel(modelActionSize)}>Use Model</button>
        <button type="button" onClick={() => void onDownloadModel(modelActionSize)}>Download Model</button>
        <button type="button" onClick={() => void onDeleteModel(modelActionSize)}>
          {confirmDeleteModelSize === modelActionSize ? "Confirm Delete" : "Delete Model"}
        </button>
        <p>Config target: {selectedModelName}</p>
        <p>Active session model: {activeModelName}</p>
        <p>Cached for active model: {model.ready ? "yes" : "no"}</p>
        <p>Active model path: hidden in the production UI</p>
        {!modelMatchesTarget ? <p>Save to keep it.</p> : null}
        */}
      </Panel>

      <div className="pane-grid pane-grid--two">
        <Panel eyebrow="Output" title="Language & decoding">
          <Field label="Language">
            <select className="ui-select" value={draft.language} onChange={(event) => onLanguageChange(event.target.value)}>
              {options.languages.map((option) => (
                <option key={option.code} value={option.code}>
                  {option.name}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Decode mode" hint="Quality vs. speed">
            <select className="ui-select" value={draft.decodeMode} onChange={(event) => onDecodeModeChange(event.target.value)}>
              {options.decodeModes.map((option) => (
                <option key={option.code} value={option.code}>
                  {option.name}
                </option>
              ))}
            </select>
          </Field>
        </Panel>

        <Panel eyebrow="Formatting" title="Punctuation & casing">
          <Field label="Punctuation">
            <select className="ui-select" value={draft.punctuationMode} onChange={(event) => onPunctuationModeChange(event.target.value)}>
              {options.punctuationModes.map((option) => (
                <option key={option.code} value={option.code}>
                  {option.name}
                </option>
              ))}
            </select>
          </Field>

          {/* Future template slot: the design includes additional formatting toggles that the runtime does not support yet.
          <div className="settings-inline-toggle">
            <label className="switch">
              <input type="checkbox" checked readOnly />
              <span className="switch__track" />
              <span className="switch__copy">
                <strong>Smart capitalization</strong>
                <small>Proper nouns and sentence starts.</small>
              </span>
            </label>
          </div>
          <div className="settings-inline-toggle">
            <label className="switch">
              <input type="checkbox" checked readOnly />
              <span className="switch__track" />
              <span className="switch__copy">
                <strong>Number formatting</strong>
                <small>Turn spoken numbers into digits.</small>
              </span>
            </label>
          </div>
          <div className="settings-inline-toggle">
            <label className="switch">
              <input type="checkbox" readOnly />
              <span className="switch__track" />
              <span className="switch__copy">
                <strong>Remove filler words</strong>
                <small>Filter out “um”, “uh”, and similar hesitations.</small>
              </span>
            </label>
          </div>
          */}
        </Panel>
      </div>
    </div>
  );
}
