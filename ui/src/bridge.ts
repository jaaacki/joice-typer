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

export function saveConfig(config: ConfigSnapshot): void {
  const handler = (window as BootstrapWindow).webkit?.messageHandlers?.joicetyper;
  if (handler === undefined) {
    throw new Error("Native settings bridge is unavailable");
  }
  handler.postMessage({
    type: "saveConfig",
    config,
  });
}
