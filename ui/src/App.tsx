import { readBootstrap } from "./bridge";
import { SettingsScreen } from "./settings/SettingsScreen";

export default function App() {
  const bootstrap = readBootstrap();

  if (bootstrap !== null) {
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
