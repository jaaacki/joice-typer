import type { AppStateSnapshot, MachineInfoSnapshot } from "../../bridge";

type AboutPaneProps = {
  activeModelName: string;
  currentAppState: AppStateSnapshot;
  machineInfo: MachineInfoSnapshot;
  runtimeStatus: string;
  saveAvailable: boolean;
};

function joinNonEmpty(values: string[]): string {
  return values.map((value) => value.trim()).filter((value) => value !== "").join(", ");
}

export default function AboutPane({ activeModelName, currentAppState, machineInfo, runtimeStatus, saveAvailable }: AboutPaneProps) {
  const graphics = joinNonEmpty(machineInfo.graphics ?? []);
  const inference = joinNonEmpty((machineInfo.inferenceBackends ?? []).map((backend) => backend.description || backend.name));

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
          {machineInfo.machineModel ? (
            <div className="about-facts__row">
              <dt>Machine</dt>
              <dd>{machineInfo.machineModel}</dd>
            </div>
          ) : null}
          {machineInfo.chip ? (
            <div className="about-facts__row">
              <dt>Chip</dt>
              <dd>{machineInfo.chip}</dd>
            </div>
          ) : null}
          {machineInfo.cpuModel && machineInfo.cpuModel !== machineInfo.chip ? (
            <div className="about-facts__row">
              <dt>CPU</dt>
              <dd>{machineInfo.cpuModel}</dd>
            </div>
          ) : null}
          {machineInfo.integratedGpu ? (
            <div className="about-facts__row">
              <dt>iGPU</dt>
              <dd>{machineInfo.integratedGpu}</dd>
            </div>
          ) : null}
          {graphics ? (
            <div className="about-facts__row">
              <dt>Graphics</dt>
              <dd>{graphics}</dd>
            </div>
          ) : null}
          {inference ? (
            <div className="about-facts__row">
              <dt>Inference</dt>
              <dd>{inference}</dd>
            </div>
          ) : null}
          {machineInfo.webViewRuntimeVersion ? (
            <div className="about-facts__row">
              <dt>WebView2</dt>
              <dd>{machineInfo.webViewRuntimeVersion}</dd>
            </div>
          ) : null}
          {machineInfo.whisperSystemInfo ? (
            <div className="about-facts__row">
              <dt>Whisper</dt>
              <dd>{machineInfo.whisperSystemInfo}</dd>
            </div>
          ) : null}
        </dl>
      </section>
    </div>
  );
}
