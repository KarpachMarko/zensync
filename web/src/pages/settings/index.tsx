import { PageHeading } from "@/components/typography";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { User } from "@/lib/types";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { ApiKeys } from "./api-keys";
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
    {
      value: "connections",
      title: "Connections",
      content: <Connections admin={user.role === "admin"} />,
    },
    {
      value: "quality",
      title: "Quality",
      content: <Quality user={user} reload={reload} />,
    },
    { value: "api", title: "API keys", content: <ApiKeys /> },
    ...(user.role === "admin"
      ? [
          {
            value: "providers",
            title: "Music providers",
            content: <MusicProviders />,
          },
          {
            value: "downloads",
            title: "Downloads",
            content: <DownloadSettings />,
          },
          { value: "users", title: "Users", content: <Users /> },
        ]
      : []),
  ];
  const segment = location.pathname.split("/")[2];
  const requested = segment === "people" ? "users" : segment;
  const activeTab = tabs.some((tab) => tab.value === requested)
    ? requested
    : "connections";
  if (segment !== activeTab)
    return <Navigate to={"/settings/" + activeTab} replace />;
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
          className="my-5 h-auto max-w-full flex-wrap justify-start gap-2 p-0 group-data-[orientation=horizontal]/tabs:h-auto"
        >
          {tabs.map((tab) => (
            <TabsTrigger
              key={tab.value}
              value={tab.value}
              className="min-h-9 flex-none px-3 py-1.5 max-[700px]:min-h-11"
            >
              {tab.title}
            </TabsTrigger>
          ))}
        </TabsList>
        {tabs.map((tab) => (
          <TabsContent key={tab.value} value={tab.value} className="text-base">
            {tab.content}
          </TabsContent>
        ))}
      </Tabs>
    </>
  );
}
