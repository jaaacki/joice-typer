import type { ReactNode } from "react";

export type PaneId = "capture" | "transcription" | "vocabulary" | "permissions" | "about";

type FieldProps = {
  label: string;
  hint?: string;
  children: ReactNode;
};

type PanelProps = {
  eyebrow?: string;
  title: string;
  right?: ReactNode;
  children: ReactNode;
};

export function Field({ label, hint, children }: FieldProps) {
  return (
    <label className="settings-field">
      <span className="settings-field__header">
        <span className="settings-field__label">{label}</span>
        {hint ? <span className="settings-field__hint">{hint}</span> : null}
      </span>
      {children}
    </label>
  );
}

export function Panel({ eyebrow, title, right, children }: PanelProps) {
  return (
    <section className="settings-panel">
      <header className="settings-panel__header">
        <div>
          {eyebrow ? <p className="settings-panel__eyebrow">{eyebrow}</p> : null}
          <h2>{title}</h2>
        </div>
        {right ? <div className="settings-panel__right">{right}</div> : null}
      </header>
      <div className="settings-panel__body">{children}</div>
    </section>
  );
}

export function StatusBadge({ tone, children }: { tone: "ok" | "warn" | "idle"; children: ReactNode }) {
  return (
    <span className={`status-badge status-badge--${tone}`}>
      <span className="status-badge__dot" />
      {children}
    </span>
  );
}

export function formatTriggerKeyDisplay(keys: string[]): string {
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

export function formatModelBytes(bytes?: number): string {
  if (bytes === undefined || bytes <= 0) {
    return "";
  }
  if (bytes >= 1_000_000_000) {
    return `${(bytes / 1_000_000_000).toFixed(1)} GB`;
  }
  return `${Math.round(bytes / 1_000_000)} MB`;
}

export function parseModelOption(option: { name: string; bytes?: number }) {
  const [titlePart, detailPart = ""] = option.name.split(" — ");
  const detailSegments = detailPart.split(" · ");
  const sizeLabel = detailSegments[0] ?? formatModelBytes(option.bytes);
  const summary = detailSegments.slice(1).join(" · ");
  return {
    title: titlePart,
    sizeLabel,
    summary,
  };
}

export function permissionsTone(granted: boolean): "ok" | "warn" {
  return granted ? "ok" : "warn";
}

export function runtimeTone(state: string): "ok" | "warn" | "idle" {
  const normalized = state.trim().toLowerCase();
  if (normalized === "ready" || normalized === "idle" || normalized === "running") {
    return "ok";
  }
  if (normalized.includes("error") || normalized.includes("failed")) {
    return "warn";
  }
  return "idle";
}
