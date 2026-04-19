import type { PermissionsSnapshot } from "../../bridge";
import { Panel, StatusBadge, permissionsTone } from "../shared";

type PermissionsPaneProps = {
  permissions: PermissionsSnapshot;
  onOpenPermissionSettings: (target: "accessibility" | "input_monitoring", label: string) => void | Promise<void>;
};

export default function PermissionsPane({ permissions, onOpenPermissionSettings }: PermissionsPaneProps) {
  return (
    <div className="pane-stack">
      <Panel eyebrow="System access" title="Permissions">
        <div className="permission-list">
          <div className="permission-item">
            <div className="permission-item__icon">
              <SparkleIcon />
            </div>
            <div className="permission-item__copy">
              <strong>Accessibility</strong>
              <span>Required to type into whichever app currently has focus.</span>
            </div>
            <StatusBadge tone={permissionsTone(permissions.accessibility)}>
              {permissions.accessibility ? "Granted" : "Needs attention"}
            </StatusBadge>
            <button
              className="ui-button ui-button--secondary"
              type="button"
              onClick={() => void onOpenPermissionSettings("accessibility", "Accessibility")}
            >
              Open
            </button>
          </div>

          <div className="permission-item">
            <div className="permission-item__icon">
              <CommandIcon />
            </div>
            <div className="permission-item__copy">
              <strong>Input Monitoring</strong>
              <span>Required to listen for the global hotkey while other apps are active.</span>
            </div>
            <StatusBadge tone={permissionsTone(permissions.inputMonitoring)}>
              {permissions.inputMonitoring ? "Granted" : "Needs attention"}
            </StatusBadge>
            <button
              className="ui-button ui-button--secondary"
              type="button"
              onClick={() => void onOpenPermissionSettings("input_monitoring", "Input Monitoring")}
            >
              Open
            </button>
          </div>

          {/* Future template slot: the preferences template also includes microphone, launch-at-login, and privacy controls.
          <div className="permission-item">
            <div className="permission-item__icon"><MicIcon /></div>
            <div className="permission-item__copy">
              <strong>Microphone</strong>
              <span>Required to capture your voice.</span>
            </div>
            <StatusBadge tone="ok">Granted</StatusBadge>
            <button className="ui-button ui-button--secondary" type="button">Manage</button>
          </div>
          */}
        </div>
      </Panel>

      {/* Future template slot: privacy toggles stay commented until the runtime grows matching config fields.
      <Panel eyebrow="Privacy" title="Data handling">
        <div className="settings-inline-toggle">On-device only</div>
        <div className="settings-inline-toggle">Share anonymous diagnostics</div>
        <div className="settings-inline-toggle">Save dictation history</div>
      </Panel>
      */}
    </div>
  );
}

function SparkleIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <path
        d="m8 2.3.9 2.8 2.8.9-2.8.9L8 9.7l-.9-2.8-2.8-.9 2.8-.9L8 2.3Zm4.1 7 .5 1.5 1.5.5-1.5.5-.5 1.5-.5-1.5-1.5-.5 1.5-.5.5-1.5Z"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function CommandIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <path
        d="M5.2 2.6a1.6 1.6 0 1 0 0 3.2h5.6a1.6 1.6 0 1 1 0 3.2H5.2a1.6 1.6 0 1 0 0 3.2m0-9.6v9.6m5.6-9.6v9.6"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.25"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
