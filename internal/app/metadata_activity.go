package app

// Keep the metadata history and the small active-job preview consistently named.
// Legacy jobs predate structured artist identities, hence the fallbacks.
const metadataActivityTitleSQL = `CASE WHEN json_valid(changes) THEN CASE
 WHEN json_type(changes,'$[0].consolidation') IS NOT NULL THEN 'Merge songs / '||json_extract(changes,'$[0].consolidation.source.title')||' → '||json_extract(changes,'$[0].song.title')
 WHEN json_extract(changes,'$[0].collectionKind')='album' THEN 'Edit album / '||json_extract(changes,'$[0].collectionName')
 WHEN json_extract(changes,'$[0].collectionKind')='artist' THEN 'Rename artist / '||json_extract(changes,'$[0].collectionName')
 ELSE CASE json_extract(changes,'$[0].mergeKind')
 WHEN 'artist' THEN 'Merge artists / '||COALESCE(json_extract(changes,'$[0].artistMerge.sourceName'),json_extract(changes,'$[0].song.artist'),'Unknown artist')||' → '||COALESCE(json_extract(changes,'$[0].artistMerge.targetName'),json_extract(changes,'$[0].edit.artist'),'Unknown artist')
 WHEN 'album' THEN 'Merge albums / '||COALESCE(json_extract(changes,'$[0].song.album'),'Unknown album')||' → '||COALESCE(json_extract(changes,'$[0].edit.album'),'Unknown album')
 WHEN 'credits' THEN 'Separate artist credits / '||COALESCE(json_extract(changes,'$[0].creditRepair.name'),json_extract(changes,'$[0].song.artist'),'Unknown artist')
 ELSE CASE WHEN json_extract(changes,'$[0].enrichment') IS NOT NULL THEN 'Fetch '||json_extract(changes,'$[0].enrichment')||' / ' ELSE 'Edit metadata / ' END||COALESCE(json_extract(changes,'$[0].edit.title'),json_extract(changes,'$[0].song.title'),'Untitled song')
 END END ELSE 'Metadata update' END`

const metadataActivityTotalSQL = `CASE WHEN json_valid(changes) THEN json_array_length(changes) ELSE 0 END`
const metadataActivityStageSQL = `CASE WHEN instr(message,printf('%d/%d',done,` + metadataActivityTotalSQL + `))=1
 THEN message ELSE printf('%d/%d · %s',done,` + metadataActivityTotalSQL + `,message) END`
