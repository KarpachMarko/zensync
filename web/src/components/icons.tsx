export function Icon({ name }: { name: string }) {
  const paths: Record<string, string[]> = {
    library: ["M4 4h16v16H4z", "M8 4v16", "M12 8h5", "M12 12h5"],
    search: ["M20 20l-5-5", "M16 10a6 6 0 1 1-12 0 6 6 0 0 1 12 0"],
    imports: ["M12 3v12", "M7 10l5 5 5-5", "M4 15v5h16v-5"],
    requests: [
      "M7 5h13",
      "M7 12h13",
      "M7 19h13",
      "M3 5h.01",
      "M3 12h.01",
      "M3 19h.01",
    ],
    activity: ["M3 12h4l3-8 4 16 3-8h4"],
    settings: [
      "M4 6h16",
      "M4 12h16",
      "M4 18h16",
      "M8 3v6",
      "M16 9v6",
      "M10 15v6",
    ],
    collapse: ["M14 6l-6 6 6 6"],
    expand: ["M10 6l6 6-6 6"],
    logout: ["M9 4H4v16h5", "M10 12h11", "M17 8l4 4-4 4"],
  };
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {(paths[name] || paths.search).map((d, i) => (
        <path key={i} d={d} />
      ))}
    </svg>
  );
}
