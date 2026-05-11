import { useState } from "react";

import { completeOnboarding, readBootstrap } from "./bridge";
import { SettingsScreen } from "./settings/SettingsScreen";

export default function App() {
  const bootstrap = readBootstrap();
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (bootstrap !== null) {
    if (bootstrap.isOnboarding) {
      const handleStart = async () => {
        setError(null);
        setSubmitting(true);
        try {
          await completeOnboarding();
        } catch (err) {
          setSubmitting(false);
          setError(err instanceof Error ? err.message : String(err));
        }
      };
      return (
        <div className="onboarding-shell">
          <header className="onboarding-banner">
            <h1>Welcome to JoiceTyper</h1>
            <p>
              Configure microphone, permissions, hotkey, and speech model below.
              When you are ready, click <strong>Start JoiceTyper</strong>.
            </p>
          </header>
          <SettingsScreen bootstrap={bootstrap} />
          <footer className="onboarding-footer">
            {error !== null ? (
              <p className="onboarding-footer__error" role="alert">
                {error}
              </p>
            ) : null}
            <button
              type="button"
              className="onboarding-footer__cta"
              onClick={handleStart}
              disabled={submitting}
            >
              {submitting ? "Starting…" : "Start JoiceTyper"}
            </button>
          </footer>
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
