import type { PermissionOptionsSnapshot, PermissionsSnapshot } from "../../bridge";
import { Panel, StatusBadge, permissionsTone } from "../shared";

type PermissionsPaneProps = {
  options: PermissionOptionsSnapshot;
  permissions: PermissionsSnapshot;
  onOpenPermissionSettings: (target: "accessibility" | "input_monitoring", label: string) => void | Promise<void>;
};

export default function PermissionsPane({ options, permissions, onOpenPermissionSettings }: PermissionsPaneProps) {
  const items = [
    {
      key: "accessibility" as const,
      label: "Accessibility",
      description: "Required to type into whichever app currently has focus.",
      requirement: options.accessibility,
      granted: permissions.accessibility,
      icon: <SparkleIcon />,
    },
    {
      key: "input_monitoring" as const,
      label: "Input Monitoring",
      description: "Required to listen for the global hotkey while other apps are active.",
      requirement: options.inputMonitoring,
      granted: permissions.inputMonitoring,
      icon: <CommandIcon />,
    },
  ].filter((item) => item.requirement.required || item.requirement.actionable);

  if (items.length === 0) {
    return (
      <div className="pane-stack">
        <Panel eyebrow="System access" title="Permissions">
          <p className="settings-panel__hint">JoiceTyper does not require additional system permission steps on this platform.</p>
        </Panel>
      </div>
    );
  }

  return (
    <div className="pane-stack">
      <Panel eyebrow="System access" title="Permissions">
        <div className="permission-list">
          {items.map((item) => (
            <div key={item.key} className="permission-item">
              <div className="permission-item__icon">{item.icon}</div>
              <div className="permission-item__copy">
                <strong>{item.label}</strong>
                <span>{item.description}</span>
              </div>
              <StatusBadge tone={permissionsTone(item.granted)}>
                {item.granted ? "Granted" : "Needs attention"}
              </StatusBadge>
              {item.requirement.actionable ? (
                <button
                  className="ui-button ui-button--secondary"
                  type="button"
                  onClick={() => void onOpenPermissionSettings(item.key, item.label)}
                >
                  Open
                </button>
              ) : null}
            </div>
          ))}

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
