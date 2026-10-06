import { useEffect, useState } from "react";

/**
 * Use bottom Action Sheet only on small screens (<768).
 * Wide desktops (even with touch) keep Grok-style side hover bar.
 */
export function useIsTouchUi(): boolean {
  const [narrow, setNarrow] = useState(() => {
    if (typeof window === "undefined") return false;
    return window.matchMedia("(max-width: 767px)").matches;
  });

  useEffect(() => {
    const mq = window.matchMedia("(max-width: 767px)");
    const sync = () => setNarrow(mq.matches);
    sync();
    mq.addEventListener("change", sync);
    return () => mq.removeEventListener("change", sync);
  }, []);

  return narrow;
}
