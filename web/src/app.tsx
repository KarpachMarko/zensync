import { AppShell } from "@/components/layout/app-shell";
import { AuthLayout } from "@/components/layout/content";
import { Text } from "@/components/typography";
import { useSession } from "@/hooks/use-session";
import { Activity } from "@/pages/activity";
import { Imports } from "@/pages/imports";
import { MigrationPage } from "@/pages/imports/migration";
import { NewImport } from "@/pages/imports/new-import";
import { AcceptInvite } from "@/pages/invite";
import { Library } from "@/pages/library";
import { Login } from "@/pages/login";
import { Requests } from "@/pages/requests";
import { Search } from "@/pages/search";
import { Settings } from "@/pages/settings";
import { useEffect } from "react";
import { Navigate, Route, Routes, useLocation } from "react-router-dom";
export function App() {
  const { setup, user, loading, error, setError, load, login, signOut } =
    useSession();
  const location = useLocation();
  useEffect(() => {
    setError("");
    if (!document.activeElement?.matches('[role="tab"]')) {
      document.querySelector<HTMLElement>("main h1")?.focus();
    }
  }, [location.pathname]);
  if (loading)
    return (
      <AuthLayout>
        <Text>Opening your workspace…</Text>
      </AuthLayout>
    );
  if (location.pathname === "/invite")
    return <AcceptInvite signedIn={!!user} onSignOut={signOut} />;
  if (!user) return <Login setup={setup} error={error} onSubmit={login} />;
  return (
    <AppShell user={user} onSignOut={signOut} onError={setError}>
      {error && <Text role="alert">{error}</Text>}
      <Routes>
        <Route path="/requests" element={<Requests user={user} />} />
        <Route
          path="/activity"
          element={<Activity admin={user.role === "admin"} />}
        />
        <Route
          path="/settings/*"
          element={<Settings user={user} reload={load} />}
        />
        <Route
          path="/library/:view"
          element={
            <Library canDelete={user.role === "admin" || user.directDelete} />
          }
        />
        <Route
          path="/search"
          element={
            <Search canDelete={user.role === "admin" || user.directDelete} />
          }
        />
        <Route path="/imports" element={<Imports />} />
        <Route path="/imports/new" element={<NewImport />} />
        <Route path="/imports/:id" element={<MigrationPage />} />
        <Route
          path="/imports/:id/songs/:sourceId"
          element={<MigrationPage />}
        />
        <Route
          path="/imports/:id/playlists/:playlistId"
          element={<MigrationPage />}
        />
        <Route path="*" element={<Navigate to="/imports" replace />} />
      </Routes>
    </AppShell>
  );
}
