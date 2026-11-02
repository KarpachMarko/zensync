import type { User } from "@/lib/types";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { DeletionKeys } from "./api-keys";
import { Connections } from "./connections";
import { DownloadSettings } from "./downloads";
import { Quality } from "./quality";
import { Users } from "./users";
export function Settings({
  user,
  reload,
}: {
  user: User;
  reload: () => Promise<void>;
}) {
  return (
    <>
      <h1 tabIndex={-1}>Settings</h1>
      <nav className="tabs" aria-label="Settings">
        <NavLink to="/settings/connections">Connections</NavLink>
        <NavLink to="/settings/quality">Quality</NavLink>
        <NavLink to="/settings/api">API keys</NavLink>
        {user.role === "admin" && (
          <NavLink to="/settings/downloads">Downloads</NavLink>
        )}
        {user.role === "admin" && <NavLink to="/settings/users">Users</NavLink>}
      </nav>
      <Routes>
        <Route
          path="connections"
          element={<Connections admin={user.role === "admin"} />}
        />
        <Route
          path="quality"
          element={<Quality user={user} reload={reload} />}
        />
        <Route path="api" element={<DeletionKeys />} />
        {user.role === "admin" && (
          <>
            <Route path="downloads" element={<DownloadSettings />} />
            <Route
              path="users"
              element={<Users user={user} reload={reload} />}
            />
            <Route path="people" element={<Navigate to="../users" replace />} />
          </>
        )}
        <Route path="*" element={<Navigate to="connections" replace />} />
      </Routes>
    </>
  );
}
