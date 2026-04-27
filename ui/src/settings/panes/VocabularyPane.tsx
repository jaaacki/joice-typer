import type { ConfigSnapshot } from "../../bridge";
import { Panel } from "../shared";

type VocabularyPaneProps = {
  draft: ConfigSnapshot;
  onVocabularyChange: (value: string) => void;
};

export default function VocabularyPane({ draft, onVocabularyChange }: VocabularyPaneProps) {
  return (
    <div className="pane-stack">
      <Panel eyebrow="Domain terms" title="Custom vocabulary">
        <p className="settings-lead">
          Bias transcription toward names, jargon, and commands you use often. Vocabulary is not applied in Whisper translation mode. Keep one term per line, or use commas if that is already how
          your config is managed.
        </p>
        <textarea
          className="ui-textarea"
          value={draft.vocabulary}
          onChange={(event) => onVocabularyChange(event.target.value)}
          placeholder="git rebase&#10;PostgreSQL&#10;TanStack"
        />
      </Panel>

      {/* Future template slot: structured replacements exist in the design, but the current config only stores raw vocabulary text.
      <Panel eyebrow="Replacements" title="Auto-corrections">
        <div className="replacement-grid">
          <input className="ui-input" value="get hub" readOnly />
          <input className="ui-input" value="GitHub" readOnly />
        </div>
      </Panel>
      */}
    </div>
  );
}
