import { ExpandableSection } from "@/components/expandable-section";
import { NewMigrationDialog } from "@/components/imports/new-migration-dialog";
import { parseAsBoolean, useQueryState } from "nuqs";
import { PageTitle, Row } from "@/components/layout/content";
import { AppLink } from "@/components/navigation";
import { StatusBadge } from "@/components/status";
import {
  Caption,
  PageHeading,
  SectionHeading,
  Text,
} from "@/components/typography";
import { Button } from "@/components/ui/button";
import { useApiQuery } from "@/hooks/use-api";
import { type Migration } from "@/lib/migration";
export function Imports() {
  const [creating, setCreating] = useQueryState(
    "new",
    parseAsBoolean.withDefault(false),
  );
  const result = useApiQuery<Migration[]>("/imports", {
    refetchInterval: 5000,
  });
  const rows = result.data ?? [];
  const completed = rows.filter(isCompleted);
  const active = rows.filter((migration) => !isCompleted(migration));
  const error = result.error?.message;
  const loading = result.isPending;
  return (
    <>
      <PageTitle>
        <PageHeading tabIndex={-1}>Imports</PageHeading>
        <Button onClick={() => void setCreating(true)}>New migration</Button>
      </PageTitle>
      <Text>
        Bring your Apple Music catalog, playlists and favorites into Navidrome.
      </Text>
      {error && <Text role="alert">{error}</Text>}
      {loading ? (
        <Text>Loading migrations…</Text>
      ) : (
        !error && (
          <>
            <section aria-labelledby="active-imports-heading">
              <SectionHeading id="active-imports-heading">
                In progress
              </SectionHeading>
              {active.length ? (
                <MigrationList migrations={active} />
              ) : (
                <Text>No imports in progress.</Text>
              )}
            </section>
            <ExpandableSection
              title="Completed imports"
              count={completed.length}
              defaultOpen={false}
            >
              {completed.length ? (
                <MigrationList migrations={completed} />
              ) : (
                <Text>No completed imports yet.</Text>
              )}
            </ExpandableSection>
          </>
        )
      )}
      {creating && (
        <NewMigrationDialog onClose={() => void setCreating(false)} />
      )}
    </>
  );
}

// Handing over ends ZenSync's management of an import in the current API lifecycle.
function isCompleted(migration: Migration) {
  return ["completed", "handed_over"].includes(migration.state);
}

function MigrationList({ migrations }: { migrations: Migration[] }) {
  return migrations.map((migration) => (
    <Row asChild key={migration.id}>
      <AppLink
        className="no-underline max-[700px]:items-start"
        to={"/imports/" + migration.id}
      >
        <div>
          <strong>{migration.name}</strong>
          <Caption>
            {migration.report.songs} songs / {migration.report.playlists}{" "}
            playlists
          </Caption>
        </div>
        <StatusBadge state={migration.state} />
      </AppLink>
    </Row>
  ));
}
