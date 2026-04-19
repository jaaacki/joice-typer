import type { AppStateSnapshot } from "../../bridge";

type AboutPaneProps = {
  activeModelName: string;
  currentAppState: AppStateSnapshot;
  runtimeStatus: string;
  saveAvailable: boolean;
};

export default function AboutPane({ activeModelName, currentAppState, runtimeStatus, saveAvailable }: AboutPaneProps) {
  return (
    <div className="pane-stack pane-stack--about">
      <section className="about-summary" aria-label="Version and runtime details">
        <p className="about-summary__lede">
          A quiet voice-to-text companion for coding. Hold your hotkey, speak naturally, and JoiceTyper types into the focused app.
        </p>

        <dl className="about-facts">
          <div className="about-facts__row">
            <dt>Version</dt>
            <dd>{currentAppState.version}</dd>
          </div>
          <div className="about-facts__row">
            <dt>Runtime</dt>
            <dd>{runtimeStatus}</dd>
          </div>
          <div className="about-facts__row">
            <dt>Active model</dt>
            <dd>{activeModelName}</dd>
          </div>
          <div className="about-facts__row">
            <dt>Bridge</dt>
            <dd>{saveAvailable ? "Connected" : "Unavailable"}</dd>
          </div>
        </dl>

        {/* Future template slot: keep the product links and update action from the prototype for later wiring.
        <div className="button-row button-row--wrap">
          <button className="ui-button ui-button--secondary" type="button">Check for updates</button>
          <button className="ui-button ui-button--secondary" type="button">Documentation</button>
          <button className="ui-button ui-button--secondary" type="button">Changelog</button>
          <button className="ui-button ui-button--secondary" type="button">Report an issue</button>
        </div>
        */}
      </section>
    </div>
  );
}
