import { PageTitle, Row } from "@/components/layout/content";
import { AppLink } from "@/components/navigation";
import { StatusBadge } from "@/components/status";
import { Caption, PageHeading, Text } from "@/components/typography";
import { Button } from "@/components/ui/button";
import { useApiQuery } from "@/hooks/use-api";
import { type Migration } from "@/lib/migration";
export function Imports() {
  const result = useApiQuery<Migration[]>("/imports");
  const rows = result.data ?? [];
  const error = result.error?.message;
  const loading = result.isPending;
  return (
    <>
      <PageTitle>
        <PageHeading tabIndex={-1}>Imports</PageHeading>
        <Button asChild>
          <AppLink to="/imports/new">New migration</AppLink>
        </Button>
      </PageTitle>
      <Text>
        Bring your Apple Music catalog, playlists and favorites into Navidrome.
      </Text>
      {error && <Text role="alert">{error}</Text>}
      {loading ? (
        <Text>Loading migrations…</Text>
      ) : rows.length === 0 ? (
        <Text className="mt-8">No imports yet.</Text>
      ) : (
        rows.map((m) => (
          <Row asChild key={m.id}>
            <AppLink
              className="max-[700px]:items-start no-underline"
              to={"/imports/" + m.id}
            >
              <div>
                <strong>{m.name}</strong>
                <Caption>
                  {m.report.songs} songs / {m.report.playlists} playlists
                </Caption>
              </div>
              <StatusBadge state={m.state} />
            </AppLink>
          </Row>
        ))
      )}
    </>
  );
}
