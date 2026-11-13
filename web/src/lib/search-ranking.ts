// Query relevance is shared across catalogs; provider identity never adds weight.
export type SearchMetadata = {
  title: string;
  artist: { name: string };
  album?: { title: string };
};

function normalize(value: string) {
  return value.normalize("NFKD").replace(/\p{M}/gu, "").toLowerCase().replace(/[^\p{L}\p{N}]+/gu, " ").trim().replace(/\s+/g, " ");
}

const versions = new Set(["live", "remix", "acoustic", "karaoke", "instrumental", "cover", "tribute", "sped", "slowed"]);

function tokenMatch(token: string, words: string[]) {
  if (words.includes(token)) return 1;
  if (token.length >= 3 && words.some(word => word.startsWith(token))) return 0.8;
  return 0;
}

export function rankSearchResults<T extends SearchMetadata>(results: readonly T[], query: string): T[] {
  const normalizedQuery = normalize(query);
  // Direct links already identify one recording/album, rather than search words.
  if (!normalizedQuery || /^(https?:|deezer:|lossless:|hitmo:)/i.test(query.trim())) return [...results];
  const tokens = [...new Set(normalizedQuery.split(" "))];
  const score = (item: T) => {
    const title = normalize(item.title);
    const artist = normalize(item.artist.name);
    const album = normalize(item.album?.title ?? "");
    const primaryWords = `${title} ${artist}`.split(" ");
    const albumWords = album.split(" ");
    const coverage = tokens.reduce((sum, token) => sum + Math.max(tokenMatch(token, primaryWords), tokenMatch(token, albumWords) * 0.35), 0) / tokens.length;
    const exact = normalizedQuery === title || normalizedQuery === `${artist} ${title}` || normalizedQuery === `${title} ${artist}`;
    const phrase = [title, artist].some(value => value === normalizedQuery || value.startsWith(normalizedQuery + " "));
    const extraVersions = title.split(" ").filter(word => versions.has(word) && !tokens.includes(word)).length;
    return coverage * 100 + (exact ? 30 : 0) + (phrase ? 10 : 0) - extraVersions * 3;
  };
  return results.map((item, index) => ({ item, index, score: score(item) }))
    .sort((a, b) => b.score - a.score || a.index - b.index)
    .map(({ item }) => item);
}
