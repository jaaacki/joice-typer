export type ConfigSnapshot = {
  triggerKey: string[];
  modelSize: string;
  language: string;
  sampleRate: number;
  soundFeedback: boolean;
  inputDevice: string;
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
};

type WebkitMessageHandler = {
  postMessage: (message: unknown) => void;
};

type NativeSaveResponse = {
  requestId: string;
  ok: boolean;
  error?: string;
};

type BootstrapWindow = Window & {
  __JOICETYPER_BOOTSTRAP__?: BootstrapPayload;
  webkit?: {
    messageHandlers?: {
      joicetyper?: WebkitMessageHandler;
    };
  };
};

export function readBootstrap(): BootstrapPayload | null {
  return (window as BootstrapWindow).__JOICETYPER_BOOTSTRAP__ ?? null;
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

export function saveConfig(config: ConfigSnapshot): Promise<void> {
  const handler = (window as BootstrapWindow).webkit?.messageHandlers?.joicetyper;
  if (handler === undefined) {
    return Promise.reject(new Error("Native settings bridge is unavailable"));
  }

  const requestId = nextRequestId();

  return new Promise<void>((resolve, reject) => {
    const onResponse = (event: Event) => {
      const detail = (event as CustomEvent<NativeSaveResponse>).detail;
      if (detail?.requestId !== requestId) {
        return;
      }
      window.removeEventListener("joicetyper-native-save", onResponse as EventListener);
      if (detail.ok) {
        resolve();
        return;
      }
      reject(new Error(detail.error ?? "Failed to save settings"));
    };

    window.addEventListener("joicetyper-native-save", onResponse as EventListener);
    handler.postMessage({
      requestId,
      type: "saveConfig",
      config,
    });
  });
}
