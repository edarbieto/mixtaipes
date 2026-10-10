import { useEffect, useState } from "react";

type Status = "checking" | "ok" | "down";

// Placeholder island: proves the frontend and the Go server talk to each other.
export default function App() {
  const [status, setStatus] = useState<Status>("checking");

  useEffect(() => {
    fetch("/healthz")
      .then((res) => setStatus(res.ok ? "ok" : "down"))
      .catch(() => setStatus("down"));
  }, []);

  return (
    <main>
      <h1>mixtaipes</h1>
      <p>Describe a moment, get its mixtape.</p>
      <p>
        Server: <strong>{status}</strong>
      </p>
    </main>
  );
}
