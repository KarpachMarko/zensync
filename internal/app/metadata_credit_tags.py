"""Format-aware artist credits for prepared metadata files. Paths arrive via stdin."""
import json
import sys
import mutagen
from mutagen.id3 import ID3, TPE1, TPE2, TXXX, TRCK, TPOS
from mutagen.mp4 import MP4Tags, MP4FreeForm

ALIASES = {
    "artist": ["artist"],
    "artists": ["artists"],
    "albumartist": ["albumartist", "album_artist", "album artist"],
    "albumartists": ["albumartists", "album artists"],
    "artistids": ["musicbrainz_artistid", "musicbrainz artist id"],
    "albumartistids": ["musicbrainz_albumartistid", "musicbrainz album artist id"],
}
FRAMES = {"artist": "TPE1", "albumartist": "TPE2"}
DESCS = {"artists": "ARTISTS", "albumartists": "ALBUM ARTISTS",
         "artistids": "MusicBrainz Artist Id", "albumartistids": "MusicBrainz Album Artist Id"}
MP4_KEYS = {"artist": "\xa9ART", "albumartist": "aART", "artists": "----:com.apple.iTunes:ARTISTS",
            "albumartists": "----:com.apple.iTunes:ALBUMARTISTS",
            "artistids": "----:com.apple.iTunes:MusicBrainz Artist Id",
            "albumartistids": "----:com.apple.iTunes:MusicBrainz Album Artist Id"}


def logical(key):
    key = key.lower()
    if key == "tpe1" or key == "\xa9art":
        return "artist"
    if key == "tpe2" or key == "aart":
        return "albumartist"
    if key.startswith("txxx:"):
        key = key[5:]
    if key.startswith("----:com.apple.itunes:"):
        key = key[len("----:com.apple.itunes:"):]
    for field, aliases in ALIASES.items():
        if key in aliases:
            return field
    return None


def load(path):
    audio = mutagen.File(path)
    if audio is None:
        raise ValueError("Unsupported file for preserving artist credits")
    if audio.tags is None:
        audio.add_tags()
    return audio


def read(audio, field):
    matches = []
    for key, value in audio.tags.items():
        if logical(key) == field:
            if hasattr(value, "text"):
                value = value.text
            values = [v.decode("utf-8") if isinstance(v, bytes) else str(v) for v in value]
            matches.append((key.lower(), values))
    if not matches:
        return []
    # Prefer native display frames, otherwise the standard (unspaced) alias.
    preferred = {"artist": ["tpe1", "\xa9art", "artist"],
                 "albumartist": ["tpe2", "aart", "albumartist", "album artist", "album_artist"]}
    order = preferred.get(field, ALIASES[field])
    matches.sort(key=lambda entry: (order.index(entry[0]) if entry[0] in order else len(order), entry[0]))
    chosen = matches[0][1]
    if any([v.casefold() for v in values] != [v.casefold() for v in chosen] for _, values in matches):
        raise ValueError("Conflicting artist tag aliases; review original tags: " + field)
    return chosen


def names(audio, plural, single, fallback):
    values = read(audio, plural)
    if values:
        return values
    values = read(audio, single)
    if len(values) > 1:
        return values
    # OpenSubsonic may expose validated individual identities even when a
    # legacy singular display tag uses a configured separator. Use those
    # identities instead of guessing how to split a display name ourselves.
    if len(fallback or []) > 1:
        return fallback
    return values or fallback or []


def write(audio, field, values):
    tags = audio.tags
    for key in list(tags.keys()):
        if logical(key) == field:
            del tags[key]
    if not values:
        return
    if isinstance(tags, ID3):
        if field in FRAMES:
            tags.add((TPE1 if field == "artist" else TPE2)(encoding=3, text=values))
        else:
            tags.add(TXXX(encoding=3, desc=DESCS[field], text=values))
    elif isinstance(tags, MP4Tags):
        key = MP4_KEYS[field]
        tags[key] = [MP4FreeForm(v.encode("utf-8")) for v in values] if key.startswith("----") else values
    else:
        # Keep all recognised aliases consistent with the native multivalue list.
        for key in ALIASES[field]:
            tags[key] = values


def replace_credit(values, ids, source, target, target_id):
    if not any(name.casefold() == source.casefold() for name in values):
        return list(values), list(ids), False
    ids = list(ids)
    if len(ids) > len(values):
        raise ValueError("Artist identities and MusicBrainz IDs cannot be aligned safely")
    ids += [""] * (len(values) - len(ids))
    result, result_ids = [], []
    changed = False
    for name, mbid in zip(values, ids):
        if name.casefold() == source.casefold():
            name, mbid, changed = target, target_id, True
        existing = next((i for i, v in enumerate(result) if name.casefold() == target.casefold() and v.casefold() == name.casefold()), None)
        if existing is None:
            result.append(name)
            result_ids.append(mbid)
        elif not result_ids[existing]:
            result_ids[existing] = mbid
    return result, result_ids if any(result_ids) else [], changed



def expand_credit(values, ids, old_name, replacements):
    if not any(name.casefold() == old_name.casefold() for name in values):
        # Navidrome can retain a combined display identity while the native
        # plural tag already contains every selected individual. Canonicalize
        # the display/aliases without splitting again or discarding their IDs.
        if all(any(value.casefold() == name.casefold() for value in values) for name in replacements):
            return list(values), list(ids)
        raise ValueError("Combined credit does not match native tags: expected " + repr(old_name) + "; found " + repr(values) + ". Review this song's artist tags")
    if len(ids) > len(values):
        if len(values) == 1:
            # Every ID belongs to the one compound credit being replaced.
            # Do not guess which ID identifies which selected artist.
            ids = []
        else:
            raise ValueError("Artist names and IDs cannot be aligned safely")
    padded = list(ids) + [""] * (len(values) - len(ids))
    names, mbids = [], []
    for name, mbid in zip(values, padded):
        if name.casefold() == old_name.casefold():
            names.extend(replacements)
            # The compound identity's MBID does not identify either individual.
            mbids.extend([""] * len(replacements))
        else:
            names.append(name)
            mbids.append(mbid)
    # Collapse only the explicitly selected identities when already credited.
    selected = {name.casefold() for name in replacements}
    result, result_ids = [], []
    for name, mbid in zip(names, mbids):
        match = next((i for i, existing in enumerate(result)
                      if name.casefold() in selected and existing.casefold() == name.casefold()), None)
        if match is None:
            result.append(name)
            result_ids.append(mbid)
        elif not result_ids[match]:
            result_ids[match] = mbid
    return result, result_ids if any(result_ids) else []

def main():
    plan = json.load(sys.stdin)
    if plan["kind"] == "edit-metadata":
        original, output = load(plan["original"]), load(plan["output"])
        changed = plan["changed"]
        cover_keys = {"covr", "metadata_block_picture", "coverart", "coverartmime"}
        mutable = {"trck": "track", "trkn": "track", "track": "track", "tracknumber": "track", "tpos": "disc", "disk": "disc", "disc": "disc", "discnumber": "disc",
                   "tit2": "title", "\xa9nam": "title", "title": "title",
                   "talb": "album", "\xa9alb": "album", "album": "album",
                   "uslt": "lyrics", "sylt": "lyrics", "\xa9lyr": "lyrics",
                   "lyrics": "lyrics", "syncedlyrics": "lyrics", "unsyncedlyrics": "lyrics"}
        for key, value in original.tags.items():
            root = key.lower().split(":")[0]
            normal = key.lower()
            if normal.startswith("txxx:"):
                normal = normal[5:]
            if normal.startswith("----:com.apple.itunes:"):
                normal = normal[len("----:com.apple.itunes:"):]
            if logical(key) is not None or root == "apic" or normal in cover_keys:
                continue
            field = mutable.get(root, mutable.get(normal))
            if field and changed.get(field):
                continue
            if "musicbrainz" in normal and ("albumid" in normal.replace(" ", "").replace("_", "") or "releasegroupid" in normal.replace(" ", "").replace("_", "")) and changed["grouping"]:
                continue
            output.tags[key] = value
        for role, fields in (("artist", {"artist", "artists", "artistids"}),
                             ("albumartist", {"albumartist", "albumartists", "albumartistids"})):
            plural = "artists" if role == "artist" else "albumartists"
            idfield = "artistids" if role == "artist" else "albumartistids"
            explicit = plan.get("artistNames" if role == "artist" else "albumArtistNames")
            if explicit is not None:
                previous = names(original, plural, role, [])
                previous_ids = read(original, idfield)
                aligned_ids = []
                for name in explicit:
                    index = next((i for i, old in enumerate(previous) if old.casefold() == name.casefold()), None)
                    aligned_ids.append(previous_ids[index] if index is not None and index < len(previous_ids) else "")
                write(output, role, [" / ".join(explicit)] if explicit else [])
                write(output, plural, explicit)
                write(output, idfield, aligned_ids if any(aligned_ids) else [])
            elif changed[role]:
                desired = plan[role]
                write(output, role, [desired] if desired else [])
                write(output, plural, [desired] if desired else [])
                write(output, idfield, [])
            else:
                for key in list(output.tags.keys()):
                    if logical(key) in fields:
                        del output.tags[key]
                for key, value in original.tags.items():
                    if logical(key) in fields:
                        output.tags[key] = value
        for field, frame, atom, canonical, aliases in (
                ("trackNumber", TRCK, "trkn", "tracknumber", {"track", "tracknumber", "tracktotal", "totaltracks"}),
                ("discNumber", TPOS, "disk", "discnumber", {"disc", "discnumber", "disctotal", "totaldiscs"})):
            number = plan.get(field, 0)
            if number <= 0:
                continue
            if isinstance(output.tags, ID3):
                output.tags.delall(frame.__name__)
                for key in list(output.tags.keys()):
                    if key.lower().removeprefix("txxx:") in aliases:
                        del output.tags[key]
                output.tags.add(frame(encoding=3, text=[str(number)]))
            elif isinstance(output.tags, MP4Tags):
                output.tags[atom] = [(number, 0)]
            else:
                for key in list(output.tags.keys()):
                    if key.lower() in aliases:
                        del output.tags[key]
                output.tags[canonical] = [str(number)]
        if isinstance(output.tags, ID3):
            output.save(v2_version=4)
        else:
            output.save()
        checked = load(plan["output"])
        for role, plural, key in (("artist", "artists", "artistNames"), ("albumartist", "albumartists", "albumArtistNames")):
            explicit = plan.get(key)
            if explicit is not None and (read(checked, plural) != explicit or read(checked, role) != ([" / ".join(explicit)] if explicit else [])):
                raise ValueError("Written individual artist fields failed verification: " + role)
        result = {}
        for role, plural, key, fallback in (("artist", "artists", "artistNames", "fallbackArtists"), ("albumartist", "albumartists", "albumArtistNames", "fallbackAlbumArtists")):
            result[key] = names(checked, plural, role, [] if changed[role] or plan.get(key) is not None else plan.get(fallback, []))
            result["artist" if role == "artist" else "albumArtist"] = (read(checked, role) or [" / ".join(result[key])])[0]
        print(json.dumps(result))
        return
    if plan["kind"] == "remove-cover":
        output = load(plan["output"])
        if isinstance(output.tags, ID3):
            output.tags.delall("APIC")
        elif isinstance(output.tags, MP4Tags):
            output.tags.pop("covr", None)
        else:
            for key in list(output.tags.keys()):
                if key.lower() in {"metadata_block_picture", "coverart", "coverartmime"}:
                    del output.tags[key]
        if hasattr(output, "clear_pictures"):
            output.clear_pictures()
        if isinstance(output.tags, ID3):
            output.save(v2_version=4)
        else:
            output.save()
        print("{}")
        return
    original, output = load(plan["original"]), load(plan["output"])
    target = original if plan["kind"] == "credits" else load(plan["target"])
    def album_values(audio):
        if isinstance(audio.tags, ID3):
            frame = audio.tags.get("TALB")
            return list(frame.text) if frame else []
        return list(audio.tags.get("\xa9alb" if isinstance(audio.tags, MP4Tags) else "album", []))
    original_album = album_values(original)
    artists = names(original, "artists", "artist", plan["fallbackArtists"])
    albumartists = names(original, "albumartists", "albumartist", plan["fallbackAlbumArtists"])
    artistids, albumartistids = read(original, "artistids"), read(original, "albumartistids")
    artist_display = (read(original, "artist") or [" / ".join(artists)])[0]
    album_display = (read(original, "albumartist") or [" / ".join(albumartists)])[0]
    if plan["kind"] == "credits":
        for repair in plan.get("creditRepairs") or [plan["creditRepair"]]:
            if repair["role"] == "artist":
                artists, artistids = expand_credit(artists, artistids, repair["name"], repair["artists"])
                artist_display = " / ".join(artists)
            else:
                albumartists, albumartistids = expand_credit(albumartists, albumartistids, repair["name"], repair["artists"])
                album_display = " / ".join(albumartists)
    elif plan["kind"] == "artist":
        source, destination = plan["sourceName"], plan["targetName"]
        if not source or not destination:
            raise ValueError("Refresh candidates and queue a new artist merge to preserve collaborators")
        target_id = ""
        target_found = False
        for plural, single, idfield, fallback in (
            ("artists", "artist", "artistids", plan["targetArtists"]),
            ("albumartists", "albumartist", "albumartistids", plan["targetAlbumArtists"]),
        ):
            target_names = names(target, plural, single, fallback)
            target_ids = read(target, idfield)
            for i, name in enumerate(target_names):
                if name.casefold() == destination.casefold():
                    target_found = True
                    if i < len(target_ids):
                        target_id = target_ids[i]
                    break
            if target_id:
                break
        if not target_found:
            raise ValueError("Destination artist is not present in the verified destination file")
        artists, artistids, changed_track = replace_credit(artists, artistids, source, destination, target_id)
        albumartists, albumartistids, changed_album = replace_credit(albumartists, albumartistids, source, destination, target_id)
        if plan["replaceTrack"] and not changed_track:
            raise ValueError("Source artist not found in original individual credits; no file changed")
        if plan["replaceAlbum"] and not changed_album:
            raise ValueError("Source album artist not found in original individual credits; no file changed")
        if not changed_track and not changed_album:
            raise ValueError("No matching artist identity in original tags; no file changed")
        if changed_track:
            artist_display = " / ".join(artists)
        if changed_album:
            album_display = " / ".join(albumartists)
    else:
        albumartists = names(target, "albumartists", "albumartist", plan["targetAlbumArtists"]) or names(target, "artists", "artist", plan["targetArtists"])
        albumartistids = read(target, "albumartistids")
        album_display = (read(target, "albumartist") or [" / ".join(albumartists)])[0]
    expected = {"artist": [artist_display] if artist_display else [], "artists": artists, "albumartist": [album_display] if album_display else [],
                "albumartists": albumartists, "artistids": artistids, "albumartistids": albumartistids}
    for field, values in expected.items():
        write(output, field, values)
    if isinstance(output.tags, ID3):
        output.save(v2_version=4)
    else:
        output.save()
    checked = load(plan["output"])
    if plan["kind"] == "credits" and album_values(checked) != original_album:
        raise ValueError("Credit repair changed the native album tag; original retained")
    for field, values in expected.items():
        if read(checked, field) != values:
            raise ValueError("Written artist credits failed verification: " + field)
    print(json.dumps({"artist": artist_display, "albumArtist": album_display, "artistNames": artists, "albumArtistNames": albumartists}))


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
