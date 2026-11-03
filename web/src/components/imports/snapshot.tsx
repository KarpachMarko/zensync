import { Subheading, Text } from "@/components/typography";
export function Snapshot({ title, value }: { title: string; value: string }) {
  let data: { name: string; ids: string[] } | null = null;
  try {
    data = JSON.parse(value);
  } catch {}
  return (
    <div>
      <Subheading>{title}</Subheading>
      {data ? (
        <>
          <Text>{data.name}</Text>
          <ol className="max-h-[180px] overflow-auto text-xs wrap-anywhere">
            {data.ids.map((id, i) => (
              <li key={i}>{id}</li>
            ))}
          </ol>
        </>
      ) : (
        <Text>No verified snapshot.</Text>
      )}
    </div>
  );
}
