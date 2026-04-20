import { useEffect, useState } from "react";
import type { LogTailSnapshot } from "../../bridge";
import { Panel, StatusBadge } from "../shared";

type LogsPaneProps = {
  fetchLogs: () => Promise<LogTailSnapshot>;
  copyVisibleLogTail: () => Promise<string>;
  copyFullLog: () => Promise<string>;
  subscribeLogsUpdated: (handler: (snapshot: LogTailSnapshot) => void) => () => void;
};

function formatByteCount(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) {
    return "0 bytes";
  }
  if (bytes >= 1_000_000_000) {
    return `${(bytes / 1_000_000_000).toFixed(1)} GB`;
  }
  if (bytes >= 1_000_000) {
    return `${(bytes / 1_000_000).toFixed(1)} MB`;
  }
  if (bytes >= 1_000) {
    return `${(bytes / 1_000).toFixed(1)} KB`;
  }
  return `${bytes.toLocaleString()} bytes`;
}

function countLines(text: string): number {
  if (text === "") {
    return 0;
  }
  const body = text.endsWith("\n") ? text.slice(0, -1) : text;
  return body === "" ? 0 : body.split("\n").length;
}

function describeError(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message !== "") {
    return error.message;
  }
  return fallback;
}

export default function LogsPane({ fetchLogs, copyVisibleLogTail, copyFullLog, subscribeLogsUpdated }: LogsPaneProps) {
  const [tail, setTail] = useState<LogTailSnapshot | null>(null);
  const [loading, setLoading] = useState(true);
  const [copyingVisible, setCopyingVisible] = useState(false);
  const [copying, setCopying] = useState(false);
  const [status, setStatus] = useState("Loading live log tail...");

  useEffect(() => {
    let cancelled = false;
    let refreshSequence = 0;

    const refreshLogs = async () => {
      const sequence = ++refreshSequence;
      try {
        const nextTail = await fetchLogs();
        if (cancelled || sequence !== refreshSequence) {
          return;
        }
        setTail(nextTail);
        setStatus("");
      } catch (error) {
        if (!cancelled && sequence === refreshSequence) {
          setStatus(describeError(error, "Failed to load logs"));
        }
      } finally {
        if (!cancelled && sequence === refreshSequence) {
          setLoading(false);
        }
      }
    };

    void refreshLogs();
    const unsubscribe = subscribeLogsUpdated(() => {
      void refreshLogs();
    });

    return () => {
      cancelled = true;
      unsubscribe();
    };
  }, [fetchLogs, subscribeLogsUpdated]);

  async function handleCopyFullLog() {
    setCopying(true);
    try {
      await copyFullLog();
      setStatus("Full log copied to clipboard.");
    } catch (error) {
      setStatus(describeError(error, "Failed to copy full log"));
    } finally {
      setCopying(false);
    }
  }

  async function handleCopyVisibleLogTail() {
    setCopyingVisible(true);
    try {
      await copyVisibleLogTail();
      setStatus("Visible log tail copied to clipboard.");
    } catch (error) {
      setStatus(describeError(error, "Failed to copy visible log tail"));
    } finally {
      setCopyingVisible(false);
    }
  }

  const lineCount = tail ? countLines(tail.text) : 0;

  return (
    <div className="pane-stack pane-stack--logs settings-logs">
      <Panel
        eyebrow="Shared diagnostics"
        title="Logs"
        right={<StatusBadge tone={tail?.truncated ? "warn" : tail ? "ok" : "idle"}>{tail ? (tail.truncated ? "Tail truncated" : "Tail current") : loading ? "Loading" : "Unavailable"}</StatusBadge>}
      >
        <div className="logs-pane">
          <div className="logs-pane__toolbar">
            <button className="ui-button ui-button--secondary" type="button" onClick={() => void handleCopyVisibleLogTail()} disabled={loading || copyingVisible}>
              {copyingVisible ? "Copying tail..." : "Copy Visible Tail"}
            </button>
            <button className="ui-button ui-button--secondary" type="button" onClick={() => void handleCopyFullLog()} disabled={loading || copying}>
              {copying ? "Copying..." : "Copy Full Log"}
            </button>
          </div>

          <textarea
            className="ui-textarea logs-pane__tail"
            readOnly
            value={tail?.text ?? ""}
            placeholder={loading ? "Loading logs..." : "No log tail available."}
            spellCheck={false}
            aria-label="Recent shared log tail"
          />

          <dl className="logs-pane__meta">
            <div className="logs-pane__meta-item">
              <dt>Lines</dt>
              <dd>{tail ? lineCount.toLocaleString() : "—"}</dd>
            </div>
            <div className="logs-pane__meta-item">
              <dt>Size</dt>
              <dd>{tail ? formatByteCount(tail.byteSize) : "—"}</dd>
            </div>
            <div className="logs-pane__meta-item">
              <dt>Updated</dt>
              <dd>{tail?.updatedAt || "Waiting for refresh"}</dd>
            </div>
          </dl>

          {status ? (
            <p className="logs-pane__status" aria-live="polite">
              {status}
            </p>
          ) : null}
        </div>
      </Panel>
    </div>
  );
}
