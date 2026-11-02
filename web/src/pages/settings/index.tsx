import { NavigationLink, TabNav } from "@/components/navigation";
import { PageHeading } from "@/components/typography";
import type { User } from "@/lib/types";
import { Navigate, Route, Routes } from "react-router-dom";
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
      <PageHeading tabIndex={-1}>Settings</PageHeading>
      <TabNav aria-label="Settings">
        <NavigationLink to="/settings/connections">Connections</NavigationLink>
        <NavigationLink to="/settings/quality">Quality</NavigationLink>
        <NavigationLink to="/settings/api">API keys</NavigationLink>
        {user.role === "admin" && (
          <NavigationLink to="/settings/downloads">Downloads</NavigationLink>
        )}
        {user.role === "admin" && (
          <NavigationLink to="/settings/users">Users</NavigationLink>
        )}
      </TabNav>
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
