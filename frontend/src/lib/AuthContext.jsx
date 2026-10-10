import React, { createContext, useState, useContext, useEffect, useCallback } from "react";
import { useNavigate, useLocation } from "react-router-dom";
import { api } from "@/lib/api";

const AuthContext = createContext();

export const AuthProvider = ({ children }) => {
  const navigate = useNavigate();
  const location = useLocation();

  const [user, setUser] = useState(null);
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isLoadingAuth, setIsLoadingAuth] = useState(true);
  const [isLoadingPublicSettings, setIsLoadingPublicSettings] = useState(true);
  const [authError, setAuthError] = useState(null);
  const [authChecked, setAuthChecked] = useState(false);
  const [configStatus, setConfigStatus] = useState(null);

  // Session state lives in an HttpOnly cookie (cft_session); the browser sends
  // it automatically, so we only need to ask the backend who we are.
  const checkUserAuth = useCallback(async () => {
    setIsLoadingAuth(true);
    try {
      const data = await api.auth.me();
      setUser(data.username || "admin");
      setIsAuthenticated(true);
      setAuthError(null);
    } catch {
      setUser(null);
      setIsAuthenticated(false);
    } finally {
      setIsLoadingAuth(false);
      setAuthChecked(true);
    }
  }, []);

  const checkAppState = useCallback(async () => {
    setIsLoadingPublicSettings(true);
    try {
      setConfigStatus(await api.auth.configStatus());
    } catch {
      setConfigStatus(null);
    } finally {
      setIsLoadingPublicSettings(false);
    }
    await checkUserAuth();
  }, [checkUserAuth]);

  useEffect(() => {
    checkAppState();
  }, [checkAppState]);

  // Any API call that gets a 401 flips us to the unauthenticated state.
  useEffect(() => {
    const onUnauthorized = () => {
      setUser(null);
      setIsAuthenticated(false);
      setAuthChecked(true);
    };
    window.addEventListener("cft:unauthorized", onUnauthorized);
    return () => window.removeEventListener("cft:unauthorized", onUnauthorized);
  }, []);

  const logout = async () => {
    try {
      await api.auth.logout();
    } catch {
      // ignore — we clear local state regardless
    }
    setUser(null);
    setIsAuthenticated(false);
    navigate("/login", { replace: true });
  };

  const navigateToLogin = useCallback(() => {
    const returnTo = location.pathname + location.search;
    const suffix = returnTo && returnTo !== "/" ? `?returnTo=${encodeURIComponent(returnTo)}` : "";
    navigate(`/login${suffix}`, { replace: true });
  }, [navigate, location]);

  return (
    <AuthContext.Provider
      value={{
        user,
        isAuthenticated,
        isLoadingAuth,
        isLoadingPublicSettings,
        authError,
        authChecked,
        configStatus,
        logout,
        navigateToLogin,
        checkUserAuth,
        checkAppState,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
};
