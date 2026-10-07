import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useSheetFormUi } from "./useIsTouchUi";

export type ConfirmOptions = {
  title: string;
  description?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
};

type ConfirmFn = (options: ConfirmOptions) => Promise<boolean>;

const ConfirmContext = createContext<ConfirmFn | null>(null);

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const [options, setOptions] = useState<ConfirmOptions | null>(null);
  const resolveRef = useRef<((value: boolean) => void) | null>(null);
  const resultRef = useRef<boolean | null>(null);
  const sheetUi = useSheetFormUi();

  const confirm = useCallback<ConfirmFn>((opts) => {
    resultRef.current = null;
    setOptions(opts);
    setOpen(true);
    return new Promise<boolean>((resolve) => {
      resolveRef.current = resolve;
    });
  }, []);

  const settle = useCallback((value: boolean) => {
    if (!resolveRef.current) return;
    const resolve = resolveRef.current;
    resolveRef.current = null;
    resultRef.current = value;
    setOpen(false);
    resolve(value);
  }, []);

  useEffect(() => {
    if (!open || !sheetUi) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") settle(false);
    };
    document.addEventListener("keydown", onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = prev;
    };
  }, [open, sheetUi, settle]);

  const cancelLabel = options?.cancelLabel ?? "取消";
  const confirmLabel = options?.confirmLabel ?? (options?.danger ? "删除" : "确认");

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      {sheetUi ? (
        open && options ? (
          <div className="action-sheet-root confirm-sheet-root" role="presentation">
            <button
              type="button"
              className="action-sheet-mask"
              aria-label="关闭"
              onClick={() => settle(false)}
            />
            <div
              className="action-sheet confirm-action-sheet"
              role="alertdialog"
              aria-modal="true"
              aria-labelledby="confirm-sheet-title"
              aria-describedby={options.description ? "confirm-sheet-desc" : undefined}
            >
              <div className="action-sheet-handle" aria-hidden />
              <div id="confirm-sheet-title" className="confirm-sheet-title">
                {options.title}
              </div>
              {options.description ? (
                <div id="confirm-sheet-desc" className="confirm-sheet-desc">
                  {options.description}
                </div>
              ) : null}
              <div className="confirm-sheet-actions">
                <button
                  type="button"
                  className="confirm-sheet-btn confirm-sheet-btn-cancel"
                  onClick={() => settle(false)}
                >
                  {cancelLabel}
                </button>
                <button
                  type="button"
                  className={`confirm-sheet-btn confirm-sheet-btn-confirm${options.danger ? " is-danger" : ""}`}
                  onClick={() => settle(true)}
                >
                  {confirmLabel}
                </button>
              </div>
            </div>
          </div>
        ) : null
      ) : (
        <AlertDialog
          open={open}
          onOpenChange={(next) => {
            if (!next) {
              // Escape / overlay / cancel path — Action sets resultRef first.
              settle(resultRef.current ?? false);
            }
          }}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{options?.title}</AlertDialogTitle>
              {options?.description ? (
                <AlertDialogDescription>{options.description}</AlertDialogDescription>
              ) : (
                <AlertDialogDescription className="sr-only">确认操作</AlertDialogDescription>
              )}
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel
                onClick={() => {
                  resultRef.current = false;
                }}
              >
                {cancelLabel}
              </AlertDialogCancel>
              <AlertDialogAction
                className={cn(options?.danger && buttonVariants({ variant: "destructive" }))}
                onClick={() => {
                  resultRef.current = true;
                }}
              >
                {confirmLabel}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      )}
    </ConfirmContext.Provider>
  );
}

export function useConfirm(): ConfirmFn {
  const ctx = useContext(ConfirmContext);
  if (!ctx) {
    throw new Error("useConfirm must be used within ConfirmProvider");
  }
  return ctx;
}
