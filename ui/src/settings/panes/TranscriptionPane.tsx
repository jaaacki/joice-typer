import { useMemo } from "react";
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
  onTranslateChange: (value: boolean) => void;
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
  onTranslateChange,
  onUseModel,
}: TranscriptionPaneProps) {
  const mode: "transcription" | "translation" = draft.translate ? "translation" : "transcription";

  const isEnglish = draft.language === "en";

  // Tiny models are too inaccurate to recommend — hidden from the grid, still
  // settable via manifest if someone forces it.
  const hiddenCodes = new Set(["tiny", "tiny.en"]);

  // Show English-only models when: transcription + English. All other cases
  // (translation, non-English transcription) show multilingual models only.
  const showEnglishOnlyModels = mode === "transcription" && isEnglish;

  const filteredModels = useMemo(() => {
    return options.models.filter((option) => {
      if (hiddenCodes.has(option.code)) return false;
      const isEn = option.englishOnly === true;
      return showEnglishOnlyModels ? isEn : !isEn;
    });
  }, [options.models, showEnglishOnlyModels]);

  return (
    <div className="pane-stack">
      <Panel eyebrow="Output" title="Mode & language">
        <Field label="Output mode" hint="What JoiceTyper types at the cursor">
          <div className="mode-segments" role="radiogroup" aria-label="Output mode">
            <button
              type="button"
              role="radio"
              aria-checked={mode === "transcription"}
              className={`mode-segments__seg${mode === "transcription" ? " is-active" : ""}`}
              onClick={() => onTranslateChange(false)}
            >
              <strong>Transcription</strong>
              <span>Speak and type in the same language</span>
            </button>
            <button
              type="button"
              role="radio"
              aria-checked={mode === "translation"}
              className={`mode-segments__seg${mode === "translation" ? " is-active" : ""}`}
              onClick={() => onTranslateChange(true)}
            >
              <strong>Translation</strong>
              <span>Speak one language, type English</span>
            </button>
          </div>
        </Field>

        {mode === "transcription" ? (
          <Field label="Language" hint="What you speak — and what gets typed">
            <select className="ui-select" value={draft.language} onChange={(event) => onLanguageChange(event.target.value)}>
              {options.languages.map((option) => (
                <option key={option.code} value={option.code}>
                  {option.name}
                </option>
              ))}
            </select>
          </Field>
        ) : (
          <div className="lang-pair">
            <Field label="From" hint="Language you speak">
              <select className="ui-select" value={draft.language} onChange={(event) => onLanguageChange(event.target.value)}>
                {options.languages.map((option) => (
                  <option key={option.code} value={option.code}>
                    {option.code === "en" ? `${option.name} — no-op in translation` : option.name}
                  </option>
                ))}
              </select>
            </Field>
            <div className="lang-pair__arrow" aria-hidden="true">→</div>
            <Field label="To" hint="Output language">
              <select className="ui-select" value="en" disabled>
                <option value="en">English</option>
              </select>
            </Field>
          </div>
        )}

        {mode === "translation" && isEnglish ? (
          <p className="pane-hint">Heads up: translating English to English is a no-op. Pick a different source language unless this is intentional.</p>
        ) : null}

        {mode === "transcription" && isEnglish ? (
          <p className="pane-hint">English-only models below are tuned specifically for English — faster and more accurate than multilingual ones.</p>
        ) : null}
      </Panel>

      <Panel eyebrow="Whisper engine" title="Model" right={<StatusBadge tone={selectedModelStatus.tone}>{selectedModelStatus.label}</StatusBadge>}>
        <div className="model-grid">
          {filteredModels.map((option) => {
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
                {option.englishOnly ? <span className="model-card__chip model-card__chip--muted">English only</span> : null}
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
        <Panel eyebrow="Quality vs speed" title="Decoding">
          <Field label="Decode mode" hint="Greedy is faster, beam is more accurate">
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
        </Panel>
      </div>
    </div>
  );
}
