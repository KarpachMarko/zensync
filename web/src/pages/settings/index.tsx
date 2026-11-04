import { PageHeading } from "@/components/typography";
import { Separator } from "@/components/ui/separator";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { User } from "@/lib/types";
import {
  Navigate,
  Route,
  Routes,
  useLocation,
  useNavigate,
} from "react-router-dom";
import { DeletionKeys } from "./api-keys";
import { Connections } from "./connections";
import { DownloadSettings } from "./downloads";
import { MusicProviders } from "./providers";
import { Quality } from "./quality";
import { Users } from "./users";
export function Settings({
  user,
  reload,
}: {
  user: User;
  reload: () => Promise<void>;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const tabs = [
    { value: "connections", title: "Connections" },
    { value: "quality", title: "Quality" },
    { value: "api", title: "API keys" },
    ...(user.role === "admin"
      ? [
          { value: "providers", title: "Music providers" },
          { value: "downloads", title: "Downloads" },
          { value: "users", title: "Users" },
        ]
      : []),
  ];
  const segment = location.pathname.split("/")[2];
  const requested = segment === "people" ? "users" : segment;
  const activeTab = tabs.some((tab) => tab.value === requested)
    ? requested
    : "connections";
  return (
    <>
      <PageHeading tabIndex={-1}>Settings</PageHeading>
      <Tabs
        value={activeTab}
        onValueChange={(value) => navigate("/settings/" + value)}
        activationMode="manual"
        className="gap-0"
      >
        <TabsList
          aria-label="Settings"
          variant="line"
          className="my-[25px] mb-2.5 h-auto flex-wrap justify-start gap-0.5 p-0 group-data-horizontal/tabs:h-auto"
        >
          {tabs.map((tab) => (
            <TabsTrigger
              key={tab.value}
              value={tab.value}
              className="min-h-9 flex-none rounded-[5px] px-2 py-1.5 text-sm font-normal text-muted-foreground data-active:bg-sidebar-accent data-active:text-foreground after:hidden max-[700px]:min-h-11"
            >
              {tab.title}
            </TabsTrigger>
          ))}
        </TabsList>
        <Separator className="mb-2.5" />
        <TabsContent value={activeTab} className="text-base">
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
                <Route path="providers" element={<MusicProviders />} />
                <Route path="downloads" element={<DownloadSettings />} />
                <Route path="users" element={<Users />} />
                <Route
                  path="people"
                  element={<Navigate to="../users" replace />}
                />
              </>
            )}
            <Route path="*" element={<Navigate to="connections" replace />} />
          </Routes>
        </TabsContent>
      </Tabs>
    </>
  );
}
