import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { Toaster } from "sonner";
import App from "./App";
import AuthCallback from "./pages/AuthCallback";
import { ConfirmProvider } from "./components/ConfirmProvider";
import "./styles.css";

const path = window.location.pathname.replace(/\/$/, "") || "/";

function Root() {
  if (path === "/auth/callback") return <AuthCallback />;
  return <App />;
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ConfirmProvider>
      <Root />
      <Toaster
        position="top-center"
        richColors
        closeButton
        theme="light"
        toastOptions={{
          classNames: {
            toast:
              "border border-[var(--border)] bg-[var(--bg-elevated)] text-[var(--text)] shadow-[var(--shadow-md)] rounded-[var(--radius-md)]",
          },
        }}
      />
    </ConfirmProvider>
  </StrictMode>,
);
