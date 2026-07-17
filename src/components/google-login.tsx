"use client";
import { useEffect, useRef, useState } from "react";
import { exchangeGoogleCredential } from "@/lib/auth";

declare global {
  interface Window {
    google?: {
      accounts: {
        id: {
          initialize: (v: { client_id: string; callback: (v: { credential: string }) => void }) => void;
          renderButton: (el: HTMLElement, v: Record<string, unknown>) => void;
        };
      };
    };
  }
}

export function GoogleLogin({
  clientId,
  onSuccess,
}: {
  clientId: string;
  onSuccess: () => void;
}) {
  const target = useRef<HTMLDivElement>(null);
  const onSuccessRef = useRef(onSuccess);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  onSuccessRef.current = onSuccess;

  useEffect(() => {
    const onCredential = async (credential: string) => {
      setBusy(true);
      setError("");
      try {
        await exchangeGoogleCredential(credential);
        onSuccessRef.current();
      } catch (err) {
        setError(err instanceof Error ? err.message : "Google sign-in failed");
      } finally {
        setBusy(false);
      }
    };

    const render = () => {
      if (!window.google || !target.current) return;
      window.google.accounts.id.initialize({
        client_id: clientId,
        callback: (response) => {
          void onCredential(response.credential);
        },
      });
      target.current.innerHTML = "";
      window.google.accounts.id.renderButton(target.current, {
        theme: "outline",
        size: "large",
        text: "signin_with",
        shape: "rectangular",
        width: 280,
      });
    };

    const existing = document.querySelector<HTMLScriptElement>('script[src="https://accounts.google.com/gsi/client"]');
    if (existing) {
      existing.addEventListener("load", render);
      render();
      return () => existing.removeEventListener("load", render);
    }
    const script = document.createElement("script");
    script.src = "https://accounts.google.com/gsi/client";
    script.async = true;
    script.onload = render;
    script.onerror = () => setError("Google Sign-In could not load");
    document.head.appendChild(script);
    return () => {
      script.onload = null;
    };
  }, [clientId]);

  return (
    <main className="login-page">
      <div className="login-card">
        <div className="brand login-brand">
          <span className="brand-mark">P</span>
          <strong>PrintStudio</strong>
        </div>
        <h1>Design something remarkable</h1>
        <p>Sign in with Google to open your personal workspace. PrintStudio exchanges your Google credential for a short-lived studio session.</p>
        <div ref={target} className="google-button" aria-busy={busy} />
        {busy && <p className="login-error">Finishing sign-in…</p>}
        {error && <p className="login-error">{error}</p>}
        <small>Authentication is provided by Google. PrintStudio never receives your Google password.</small>
      </div>
    </main>
  );
}
