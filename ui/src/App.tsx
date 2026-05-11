import { readBootstrap } from "./bridge";
import { SettingsScreen } from "./settings/SettingsScreen";

export default function App() {
  const bootstrap = readBootstrap();

  if (bootstrap !== null) {
    if (bootstrap.isOnboarding) {
      return (
        <div className="onboarding-shell">
          <header className="onboarding-banner">
            <h1>Welcome to JoiceTyper</h1>
            <p>
              Configure microphone, permissions, hotkey, and speech model below.
              Close this window when you are ready to start.
            </p>
          </header>
          <SettingsScreen bootstrap={bootstrap} />
        </div>
      );
    }
    return <SettingsScreen bootstrap={bootstrap} />;
  }

  return (
    <main className="app-shell">
      <section className="hero">
        <p className="eyebrow">Embedded UI Shell</p>
        <h1>JoiceTyper</h1>
        <p className="body">
          React and TypeScript will drive the shared desktop UI from here.
        </p>
      </section>
    </main>
  );
}
