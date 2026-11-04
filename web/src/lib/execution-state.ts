export const finished = (state: string) =>
  [
    "available",
    "completed",
    "cancelled",
    "declined",
    "withdrawn",
    "superseded",
  ].includes(state);
export const blocked = (state: string) =>
  ["blocked", "failed", "deferred"].includes(state);
export const executionSections = [
  "In progress",
  "Blocked",
  "Completed",
  "Queued",
] as const;
export function executionGroup(state: string) {
  if (state === "awaiting_approval") return "Requested";
  if (state === "queued") return "Queued";
  if (finished(state)) return "Completed";
  if (blocked(state)) return "Blocked";
  return "In progress";
}
function executionPriority(state: string) {
  if (state === "downloading") return 0;
  if (["awaiting_scan", "scanning"].includes(state)) return 1;
  if (state === "importing") return 2;
  if (blocked(state)) return 4;
  if (state === "awaiting_approval") return 4.5;
  if (state === "queued") return 5;
  if (finished(state)) return 6;
  return 3;
}
export function sortExecutionRows<T>(
  rows: T[],
  state: (row: T) => string,
): T[] {
  return [...rows].sort(
    (a, b) => executionPriority(state(a)) - executionPriority(state(b)),
  );
}
