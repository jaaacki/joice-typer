import {
  BRIDGE_EVENT_NAME,
  ERROR_CODES,
  EVENTS,
  KINDS,
  METHODS,
  PROTOCOL_VERSION,
  type BridgeError,
  type BridgeErrorCode,
  type BridgeEventEnvelope,
  type BridgeEventName,
  type BridgeMethod,
  type BridgeRequestEnvelope,
  type BridgeResponseEnvelope,
} from "./generated/protocol";

export type ConfigSnapshot = {
  triggerKey: string[];
  modelSize: string;
  language: string;
  sampleRate: number;
  soundFeedback: boolean;
  inputDevice: string;
  inputDeviceName: string;
  decodeMode: string;
  punctuationMode: string;
  vocabulary: string;
};

export type AppStateSnapshot = {
  state: string;
  version: string;
};

export type BootstrapPayload = {
  config: ConfigSnapshot;
  appState: AppStateSnapshot;
  permissions: PermissionsSnapshot;
  model: ModelSnapshot;
  options: SettingsOptionsSnapshot;
};

export type PermissionsSnapshot = {
  accessibility: boolean;
  inputMonitoring: boolean;
};

export type DeviceSnapshot = {
  id: string;
  name: string;
  isDefault: boolean;
};

export type ModelSnapshot = {
  size: string;
  path: string;
  ready: boolean;
};

export type ModelDownloadProgressSnapshot = {
  size: string;
  progress: number;
  bytesDownloaded: number;
  bytesTotal: number;
};

export type ModelDownloadResultSnapshot = {
  size: string;
};

export type ModelDownloadFailedSnapshot = {
  size: string;
  message: string;
  retriable: boolean;
};

export type OptionSnapshot = {
  code: string;
  name: string;
  bytes?: number;
  installed: boolean;
};

export type SettingsOptionsSnapshot = {
  models: OptionSnapshot[];
  languages: OptionSnapshot[];
  decodeModes: OptionSnapshot[];
  punctuationModes: OptionSnapshot[];
  permissions: PermissionOptionsSnapshot;
  hotkey: HotkeyOptionsSnapshot;
};

export type PermissionOptionsSnapshot = {
  accessibility: PermissionRequirementSnapshot;
  inputMonitoring: PermissionRequirementSnapshot;
};

export type PermissionRequirementSnapshot = {
  required: boolean;
  actionable: boolean;
};

export type HotkeyOptionsSnapshot = {
  modifiers: string[];
  keys: string[];
};

export type HotkeyCaptureSnapshot = {
  triggerKey: string[];
  display: string;
  recording: boolean;
  canConfirm: boolean;
};

export type InputLevelSnapshot = {
  level: number;
  quality: string;
};

type WebkitMessageHandler = {
  postMessage: (message: unknown) => void;
};

type BootstrapWindow = Window & {
  __JOICETYPER_BOOTSTRAP__?: BridgeResponseEnvelope<BootstrapPayload>;
  webkit?: {
    messageHandlers?: {
      joicetyper?: WebkitMessageHandler;
    };
  };
};

const REQUEST_TIMEOUT_MS = 15000;

export class BridgeRequestError extends Error {
  readonly code: BridgeErrorCode;
  readonly details: Record<string, unknown>;
  readonly retriable: boolean;

  constructor(error: BridgeError) {
    super(error.message);
    this.name = "BridgeRequestError";
    this.code = error.code;
    this.details = error.details;
    this.retriable = error.retriable;
  }
}

function isBridgeError(value: unknown): value is BridgeError {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const candidate = value as Partial<BridgeError>;
  return typeof candidate.code === "string" && typeof candidate.message === "string" && typeof candidate.retriable === "boolean";
}

function normalizeBridgeError(error: unknown, fallbackMessage: string, fallbackDetails: Record<string, unknown>): BridgeError {
  if (isBridgeError(error)) {
    return {
      code: error.code,
      message: error.message,
      details: typeof error.details === "object" && error.details !== null ? error.details : {},
      retriable: error.retriable,
    };
  }
  return {
    code: ERROR_CODES.internalError,
    message: fallbackMessage,
    details: fallbackDetails,
    retriable: false,
  };
}

function isBridgeResponseEnvelope<TResult>(value: unknown): value is BridgeResponseEnvelope<TResult> {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const candidate = value as Partial<BridgeResponseEnvelope<TResult>>;
  return candidate.v === PROTOCOL_VERSION && candidate.kind === KINDS.response && typeof candidate.id === "string" && typeof candidate.ok === "boolean";
}

function isBridgeEventEnvelope<TPayload>(value: unknown): value is BridgeEventEnvelope<TPayload> {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const candidate = value as Partial<BridgeEventEnvelope<TPayload>>;
  return candidate.v === PROTOCOL_VERSION && candidate.kind === KINDS.event && typeof candidate.event === "string";
}

export function readBootstrap(): BootstrapPayload | null {
  const envelope = (window as BootstrapWindow).__JOICETYPER_BOOTSTRAP__;
  if (isBridgeResponseEnvelope<BootstrapPayload>(envelope) && envelope.ok === true) {
    return envelope.result ?? null;
  }
  return null;
}

export function canPostNativeMessage(): boolean {
  return typeof (window as BootstrapWindow).webkit?.messageHandlers?.joicetyper?.postMessage === "function";
}

function nextRequestId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `req-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function nativeHandler(): WebkitMessageHandler | undefined {
  return (window as BootstrapWindow).webkit?.messageHandlers?.joicetyper;
}

function missingBridgeError(): BridgeRequestError {
  return new BridgeRequestError({
    code: ERROR_CODES.internalError,
    message: "Native settings bridge is unavailable",
    details: {},
    retriable: false,
  });
}

function query<TResult, TParams extends Record<string, unknown>>(method: BridgeMethod, params: TParams): Promise<TResult> {
  const handler = nativeHandler();
  if (handler === undefined) {
    return Promise.reject(missingBridgeError());
  }

  const requestId = nextRequestId();

  return new Promise<TResult>((resolve, reject) => {
    const cleanup = (timeoutId?: number) => {
      window.removeEventListener(BRIDGE_EVENT_NAME, onResponse as EventListener);
      if (timeoutId !== undefined) {
        window.clearTimeout(timeoutId);
      }
    };

    const onResponse = (event: Event) => {
      const detail = (event as CustomEvent<unknown>).detail;
      if (!isBridgeResponseEnvelope<TResult>(detail) || detail.id !== requestId) {
        return;
      }
      cleanup(timeoutId);
      if (detail.ok) {
        resolve(detail.result as TResult);
        return;
      }
      reject(
        new BridgeRequestError(
          normalizeBridgeError(detail.error, `Bridge request failed: ${method}`, {
            method,
            requestId,
          }),
        ),
      );
    };

    const timeoutId = window.setTimeout(() => {
      cleanup(timeoutId);
      reject(new BridgeRequestError({
        code: ERROR_CODES.internalError,
        message: `Native settings bridge timed out: ${method}`,
        details: { method, requestId },
        retriable: true,
      }));
    }, REQUEST_TIMEOUT_MS);

    const request: BridgeRequestEnvelope<TParams> = {
      v: PROTOCOL_VERSION,
      kind: KINDS.request,
      id: requestId,
      method,
      params,
    };
    window.addEventListener(BRIDGE_EVENT_NAME, onResponse as EventListener);
    try {
      handler.postMessage(request);
    } catch (error) {
      cleanup(timeoutId);
      reject(
        new BridgeRequestError({
          code: ERROR_CODES.internalError,
          message: `Native settings bridge failed to send request: ${method}`,
          details: {
            method,
            requestId,
            cause: error instanceof Error ? error.message : String(error),
          },
          retriable: true,
        }),
      );
    }
  });
}

function subscribeEvent<TPayload>(eventName: BridgeEventName, handler: (payload: TPayload) => void): () => void {
  const onEvent = (event: Event) => {
    const detail = (event as CustomEvent<unknown>).detail;
    if (!isBridgeEventEnvelope<TPayload>(detail) || detail.event !== eventName) {
      return;
    }
    handler(detail.payload);
  };

  window.addEventListener(BRIDGE_EVENT_NAME, onEvent as EventListener);
  return () => {
    window.removeEventListener(BRIDGE_EVENT_NAME, onEvent as EventListener);
  };
}

export function fetchConfig(): Promise<ConfigSnapshot> {
  return query<ConfigSnapshot, Record<string, never>>(METHODS.configGet, {});
}

export function fetchPermissions(): Promise<PermissionsSnapshot> {
  return query<PermissionsSnapshot, Record<string, never>>(METHODS.permissionsGet, {});
}

export function fetchDevices(): Promise<DeviceSnapshot[]> {
  return query<DeviceSnapshot[], Record<string, never>>(METHODS.devicesList, {});
}

export function refreshDevices(): Promise<DeviceSnapshot[]> {
  return query<{ devices: DeviceSnapshot[] }, Record<string, never>>(METHODS.devicesRefresh, {}).then((result) => result.devices);
}

export function fetchModel(): Promise<ModelSnapshot> {
  return query<ModelSnapshot, Record<string, never>>(METHODS.modelGet, {});
}

export function fetchRuntime(): Promise<AppStateSnapshot> {
  return query<AppStateSnapshot, Record<string, never>>(METHODS.runtimeGet, {});
}

export function fetchLogs(): Promise<LogTailSnapshot> {
  return query<LogTailSnapshot, Record<string, never>>(METHODS.logsGet, {});
}

export function copyVisibleLogTail(): Promise<string> {
  return query<string, Record<string, never>>(METHODS.logsCopyTail, {});
}

export function copyFullLog(): Promise<string> {
  return query<string, Record<string, never>>(METHODS.logsCopyAll, {});
}

export function fetchOptions(): Promise<SettingsOptionsSnapshot> {
  return query<SettingsOptionsSnapshot, Record<string, never>>(METHODS.optionsGet, {});
}

export function openPermissionSettings(target: "accessibility" | "input_monitoring"): Promise<void> {
  return query<{ opened: boolean }, { target: "accessibility" | "input_monitoring" }>(METHODS.permissionsOpenSettings, {
    target,
  }).then(() => undefined);
}

export function saveConfig(config: ConfigSnapshot): Promise<void> {
  return query<{ saved: boolean }, { config: ConfigSnapshot }>(METHODS.configSave, { config }).then(() => undefined);
}

export function downloadModel(size: string): Promise<void> {
  return query<{ size: string }, { size: string }>(METHODS.modelDownload, { size }).then(() => undefined);
}

export function deleteModel(size: string): Promise<void> {
  return query<{ size: string }, { size: string }>(METHODS.modelDelete, { size }).then(() => undefined);
}

export function useModel(size: string): Promise<void> {
  return query<{ size: string }, { size: string }>(METHODS.modelUse, { size }).then(() => undefined);
}

export function startHotkeyCapture(): Promise<HotkeyCaptureSnapshot> {
  return query<HotkeyCaptureSnapshot, Record<string, never>>(METHODS.hotkeyCaptureStart, {});
}

export function cancelHotkeyCapture(): Promise<void> {
  return query<{ cancelled: boolean }, Record<string, never>>(METHODS.hotkeyCaptureCancel, {}).then(() => undefined);
}

export function confirmHotkeyCapture(): Promise<HotkeyCaptureSnapshot> {
  return query<HotkeyCaptureSnapshot, Record<string, never>>(METHODS.hotkeyCaptureConfirm, {});
}

export function setAudioInputMonitor(inputDevice: string): Promise<void> {
  return query<{ selected: boolean }, { inputDevice: string }>(METHODS.audioInputMonitorSet, { inputDevice }).then(() => undefined);
}

export function subscribeRuntimeStateChanged(handler: (appState: AppStateSnapshot) => void): () => void {
  return subscribeEvent(EVENTS.runtimeStateChanged, handler);
}

export function subscribePermissionsChanged(handler: (permissions: PermissionsSnapshot) => void): () => void {
  return subscribeEvent(EVENTS.permissionsChanged, handler);
}

export function subscribeModelChanged(handler: (model: ModelSnapshot) => void): () => void {
  return subscribeEvent(EVENTS.modelChanged, handler);
}

export function subscribeModelDownloadProgress(handler: (progress: ModelDownloadProgressSnapshot) => void): () => void {
  return subscribeEvent(EVENTS.modelDownloadProgress, handler);
}

export function subscribeModelDownloadCompleted(handler: (snapshot: ModelDownloadResultSnapshot) => void): () => void {
  return subscribeEvent(EVENTS.modelDownloadCompleted, handler);
}

export function subscribeModelDownloadFailed(handler: (snapshot: ModelDownloadFailedSnapshot) => void): () => void {
  return subscribeEvent(EVENTS.modelDownloadFailed, handler);
}

export function subscribeConfigSaved(handler: (config: ConfigSnapshot) => void): () => void {
  return subscribeEvent(EVENTS.configSaved, handler);
}

export function subscribeDevicesChanged(handler: (devices: DeviceSnapshot[]) => void): () => void {
  return subscribeEvent(EVENTS.devicesChanged, handler);
}

export function subscribeHotkeyCaptureChanged(handler: (snapshot: HotkeyCaptureSnapshot) => void): () => void {
  return subscribeEvent(EVENTS.hotkeyCaptureChanged, handler);
}

export function subscribeInputLevelChanged(handler: (snapshot: InputLevelSnapshot) => void): () => void {
  return subscribeEvent(EVENTS.inputLevelChanged, handler);
}

export function subscribeLogsUpdated(handler: (snapshot: LogTailSnapshot) => void): () => void {
  return subscribeEvent(EVENTS.logsUpdated, handler);
}
