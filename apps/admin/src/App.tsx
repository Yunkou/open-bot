import type { ReactNode } from "react";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { Spin } from "antd";
import { AuthProvider, useAuth } from "./auth/AuthContext";
import AdminLayout from "./layouts/AdminLayout";
import LoginPage from "./pages/LoginPage";
import DeniedPage from "./pages/DeniedPage";
import MembersPage from "./pages/MembersPage";
import UsersPage from "./pages/UsersPage";
import BotsPage from "./pages/BotsPage";
import TracesPage from "./pages/TracesPage";
import LLMPage from "./pages/LLMPage";
import UsagePage from "./pages/UsagePage";
import FlagsPage from "./pages/FlagsPage";
import AuditPage from "./pages/AuditPage";
import { getToken } from "./api";

function RequireAdmin({ children }: { children: ReactNode }) {
  const { user, canAdmin, denied, loading } = useAuth();

  if (loading) {
    return (
      <div style={{ minHeight: "100vh", display: "grid", placeItems: "center" }}>
        <Spin size="large" tip="加载中…" />
      </div>
    );
  }

  if (!getToken() || !user) {
    const qs = window.location.search;
    return <Navigate to={`/login${qs}`} replace />;
  }

  if (denied || !canAdmin) {
    return <DeniedPage />;
  }

  return <>{children}</>;
}

function PublicOnly({ children }: { children: ReactNode }) {
  const { user, canAdmin, loading } = useAuth();

  if (loading) {
    return (
      <div style={{ minHeight: "100vh", display: "grid", placeItems: "center" }}>
        <Spin size="large" tip="加载中…" />
      </div>
    );
  }

  if (getToken() && user && canAdmin) {
    return <Navigate to="/users" replace />;
  }

  // Non-admin leftover session: show denial instead of login form.
  if (user && !canAdmin) {
    return <DeniedPage />;
  }

  return <>{children}</>;
}

function AppRoutes() {
  return (
    <Routes>
      <Route
        path="/login"
        element={
          <PublicOnly>
            <LoginPage />
          </PublicOnly>
        }
      />
      <Route
        path="/"
        element={
          <RequireAdmin>
            <AdminLayout />
          </RequireAdmin>
        }
      >
        <Route index element={<Navigate to="/users" replace />} />
        <Route path="users" element={<UsersPage />} />
        <Route path="bots" element={<BotsPage />} />
        <Route path="traces" element={<TracesPage />} />
        <Route path="members" element={<MembersPage />} />
        <Route path="llm" element={<LLMPage />} />
        <Route path="usage" element={<UsagePage />} />
        <Route path="flags" element={<FlagsPage />} />
        <Route path="audit" element={<AuditPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </BrowserRouter>
  );
}
