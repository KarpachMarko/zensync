import { createSerializer, parseAsString } from "nuqs";

export const searchParsers = { q: parseAsString.withDefault("") };
export const searchHref = (q: string) =>
  createSerializer(searchParsers)("/search", { q });
