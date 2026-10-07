import { useEffect, useState } from "react";

const POINTER_MQ = "(pointer: coarse)";
const NARROW_MQ = "(max-width: 767px)";
/** Master-detail / create-bot sheet breakpoint (orthogonal to touch message UI). */
const SHEET_FORM_MQ = "(max-width: 860px)";

const TOUCH_UI_CLASS = "touch-ui";

function readTouchUi(): boolean {
  if (typeof window === "undefined") return false;
  return (
    window.matchMedia(POINTER_MQ).matches ||
    window.matchMedia(NARROW_MQ).matches
  );
}

function syncTouchUiClass(on: boolean) {
  if (typeof document === "undefined") return;
  document.documentElement.classList.toggle(TOUCH_UI_CLASS, on);
}

/**
 * Touch / phone message UI: Action Sheet, no hover-only bars.
 * `pointer: coarse` OR width &lt;768. Wide touch tablets get the sheet.
 * Also toggles `html.touch-ui` so CSS hides hover bars with the same rule.
 */
export function useIsTouchUi(): boolean {
  const [touch, setTouch] = useState(readTouchUi);

  useEffect(() => {
    const pointerMq = window.matchMedia(POINTER_MQ);
    const narrowMq = window.matchMedia(NARROW_MQ);
    const sync = () => {
      const next = pointerMq.matches || narrowMq.matches;
      setTouch(next);
      syncTouchUiClass(next);
    };
    sync();
    pointerMq.addEventListener("change", sync);
    narrowMq.addEventListener("change", sync);
    return () => {
      pointerMq.removeEventListener("change", sync);
      narrowMq.removeEventListener("change", sync);
    };
  }, []);

  return touch;
}

/**
 * Create Bot / settings form: full-width bottom sheet when touch OR width ≤860.
 */
export function useSheetFormUi(): boolean {
  const touch = useIsTouchUi();
  const [narrow, setNarrow] = useState(() => {
    if (typeof window === "undefined") return false;
    return window.matchMedia(SHEET_FORM_MQ).matches;
  });

  useEffect(() => {
    const mq = window.matchMedia(SHEET_FORM_MQ);
    const sync = () => setNarrow(mq.matches);
    sync();
    mq.addEventListener("change", sync);
    return () => mq.removeEventListener("change", sync);
  }, []);

  return touch || narrow;
}
