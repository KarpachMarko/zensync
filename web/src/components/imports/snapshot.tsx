export function Snapshot({ title, value }: { title: string; value: string }) {
  let data: { name: string; ids: string[] } | null = null;
  try {
    data = JSON.parse(value);
  } catch {}
  return (
    <div>
      <h3>{title}</h3>
      {data ? (
        <>
          <p>{data.name}</p>
          <ol className="snapshot">
            {data.ids.map((id, i) => (
              <li key={i}>{id}</li>
            ))}
          </ol>
        </>
      ) : (
        <p>No verified snapshot.</p>
      )}
    </div>
  );
}
