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

type BootstrapWindow = Window & {
  __JOICETYPER_BOOTSTRAP__?: BootstrapPayload;
};

export function readBootstrap(): BootstrapPayload | null {
  return (window as BootstrapWindow).__JOICETYPER_BOOTSTRAP__ ?? null;
}
