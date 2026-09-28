import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import {
  User,
  clearSession,
  fetchMe,
  getStoredUser,
  getToken,
  setSession,
  adminGetOrg,
} from "../api";

type AuthContextValue = {
  user: User | null;
  orgName: string;
  denied: string;
  loading: boolean;
  canAdmin: boolean;
  setUser: (u: User | null) => void;
  setDenied: (msg: string) => void;
  logout: () => void;
  refreshMe: () => Promise<User | null>;
  loadOrg: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

function isAdminRole(role?: string): boolean {
  return role === "org_admin" || role === "platform_admin";
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(() => getStoredUser());
  const [orgName, setOrgName] = useState("");
  const [orgLoaded, setOrgLoaded] = useState(false);
  const [denied, setDenied] = useState("");
  const [loading, setLoading] = useState(() => Boolean(getToken()));

  const canAdmin = Boolean(user && isAdminRole(user.role));

  // Drop JWT when a non-admin somehow has an admin-app session (e.g. old chat login).
  useEffect(() => {
    if (user && !isAdminRole(user.role) && getToken()) {
      clearSession();
    }
  }, [user]);

  const refreshMe = useCallback(async (): Promise<User | null> => {
    if (!getToken()) {
      setUser(null);
      return null;
    }
    try {
      const me = await fetchMe();
      setUser(me);
      setSession(getToken()!, me);
      return me;
    } catch {
      clearSession();
      setUser(null);
      return null;
    }
  }, []);

  const loadOrg = useCallback(async () => {
    try {
      const data = await adminGetOrg();
      setOrgName(data.org?.name || data.org?.slug || "");
      if (data.me) setUser(data.me);
      setDenied("");
    } catch (err) {
      setDenied(err instanceof Error ? err.message : String(err));
    } finally {
      setOrgLoaded(true);
    }
  }, []);

  // Session restore on mount.
  useEffect(() => {
    if (!getToken()) {
      setLoading(false);
      return;
    }
    setLoading(true);
    void refreshMe()
      .then((me) => {
        if (me && isAdminRole(me.role)) return loadOrg();
      })
      .finally(() => setLoading(false));
  }, [refreshMe, loadOrg]);

  // After password/OIDC login, user is set without remount — load org then.
  useEffect(() => {
    if (!getToken() || !user || !isAdminRole(user.role)) return;
    if (orgLoaded || denied) return;
    void loadOrg();
  }, [user, orgLoaded, denied, loadOrg]);

  const logout = useCallback(() => {
    clearSession();
    setUser(null);
    setDenied("");
    setOrgName("");
    setOrgLoaded(false);
  }, []);

  const value = useMemo(
    () => ({
      user,
      orgName,
      denied,
      loading,
      canAdmin,
      setUser,
      setDenied,
      logout,
      refreshMe,
      loadOrg,
    }),
    [user, orgName, denied, loading, canAdmin, logout, refreshMe, loadOrg],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
