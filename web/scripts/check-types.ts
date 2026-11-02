import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";

function check(path: string): number {
  let failures = 0;
  const source = ts.createSourceFile(
    path,
    readFileSync(path, "utf8"),
    ts.ScriptTarget.Latest,
    true,
  );
  function visit(node: ts.Node): void {
    if (node.kind === ts.SyntaxKind.AnyKeyword) {
      const position = source.getLineAndCharacterOfPosition(
        node.getStart(source),
      );
      console.error(
        `${path}:${position.line + 1}: Avoid the explicit any type.`,
      );
      failures++;
    }
    ts.forEachChild(node, visit);
  }
  visit(source);
  return failures;
}
function walk(directory: string): number {
  return readdirSync(directory, { withFileTypes: true }).reduce(
    (total, entry) => {
      if (
        ["node_modules", "dist", "test-results", "playwright-report"].includes(
          entry.name,
        )
      )
        return total;
      const path = join(directory, entry.name);
      return (
        total +
        (entry.isDirectory()
          ? walk(path)
          : /\.tsx?$/.test(path)
            ? check(path)
            : 0)
      );
    },
    0,
  );
}
if (walk(".")) process.exitCode = 1;
