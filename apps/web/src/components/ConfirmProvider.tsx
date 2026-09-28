import {
  createContext,
  useCallback,
  useContext,
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

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
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
              {options?.cancelLabel ?? "取消"}
            </AlertDialogCancel>
            <AlertDialogAction
              className={cn(options?.danger && buttonVariants({ variant: "destructive" }))}
              onClick={() => {
                resultRef.current = true;
              }}
            >
              {options?.confirmLabel ?? (options?.danger ? "删除" : "确认")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
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
