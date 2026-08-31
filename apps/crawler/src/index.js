// Spotify hip-hop album crawler for hiphouse (힙집).
// Roster-driven: you curate artist IDs in artists.txt; this pulls each artist's albums by ID
// into a local Postgres DB (see docker-compose.yml). Spotify has no album-level genre, so a
// hand-picked roster IS the genre filter. Node native fetch; pg for Postgres.
//
// Auth: Client Credentials (keys in .env, see .env.example).
// Run:  docker compose up -d db  &&  node --env-file-if-exists=.env src/index.js

import pg from "pg";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";

const TOKEN_URL = "https://accounts.spotify.com/api/token";
const API = "https://api.spotify.com/v1";
const MARKET = process.env.MARKET ?? "KR";
const GROUPS = "album,single"; // ponytail: EPs land here as single/album — album_type stored, filter in UI
const DATABASE_URL = process.env.DATABASE_URL ?? "postgres://hiphouse:hiphouse@localhost:5432/hiphouse";

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// ---- pure helpers (self-checked in test.js) ----
export function parseArtistId(line) {
  const s = line.split("#")[0].trim();
  if (!s) return null;
  const m = s.match(/artist[:/]([0-9A-Za-z]{22})/) ?? s.match(/^([0-9A-Za-z]{22})$/);
  return m ? m[1] : null;
}

export const yearOf = (releaseDate) => {
  const y = parseInt((releaseDate ?? "").slice(0, 4), 10);
  return Number.isInteger(y) ? y : null;
};

export function toAlbumRow(album) {
  return {
    id: album.id,
    name: album.name,
    release_date: album.release_date ?? null,
    year: yearOf(album.release_date),
    album_type: album.album_type ?? null,
    total_tracks: album.total_tracks ?? null,
    image_url: album.images?.[0]?.url ?? null,
    spotify_url: album.external_urls?.spotify ?? null,
  };
}

// Every credited artist on the album, in Spotify's order, with real Spotify IDs.
export function albumArtists(album) {
  return (album.artists ?? [])
    .filter((a) => a?.id)
    .map((a, position) => ({ id: a.id, name: a.name ?? null, position }));
}

// ---- Spotify ----

// Which credential pair to use: --key=first / --key=second (or SPOTIFY_KEY env).
// No key → unprefixed SPOTIFY_CLIENT_* first, then FIRST_-prefixed as fallback.
export function credKey() {
  const arg = process.argv.find((a) => a.startsWith("--key="));
  return (arg ? arg.slice(6) : (process.env.SPOTIFY_KEY ?? "")).trim().toLowerCase();
}

function resolveCreds(key = credKey()) {
  const prefixes = key ? [key.toUpperCase() + "_"] : ["", "FIRST_"];
  for (const p of prefixes) {
    const id = process.env[p + "SPOTIFY_CLIENT_ID"];
    const secret = process.env[p + "SPOTIFY_CLIENT_SECRET"];
    if (id && secret) {
      console.error(`spotify creds: ${p ? p + "SPOTIFY_CLIENT_*" : "SPOTIFY_CLIENT_* (default)"}`);
      return { id, secret };
    }
  }
  const want = key ? `${key.toUpperCase()}_SPOTIFY_CLIENT_ID / _SECRET` : "SPOTIFY_CLIENT_ID / SPOTIFY_CLIENT_SECRET (or FIRST_-prefixed)";
  console.error(`Missing Spotify credentials for key "${key || "default"}" — set ${want} in .env.`);
  console.error("Create a free app at https://developer.spotify.com/dashboard, copy .env.example to .env, fill it in.");
  process.exit(1);
}

export async function getToken() {
  const { id, secret } = resolveCreds();
  const res = await fetch(TOKEN_URL, {
    method: "POST",
    headers: {
      Authorization: "Basic " + Buffer.from(`${id}:${secret}`).toString("base64"),
      "Content-Type": "application/x-www-form-urlencoded",
    },
    body: "grant_type=client_credentials",
  });
  if (!res.ok) throw new Error(`token ${res.status}: ${await res.text()}`);
  return (await res.json()).access_token;
}

export async function spFetch(url, tokenRef) {
  let refreshed = false;
  for (;;) {
    const res = await fetch(url, { headers: { Authorization: `Bearer ${tokenRef.value}` } });
    if (res.status === 429) {
      const wait = parseInt(res.headers.get("retry-after") ?? "1", 10) + 1;
      // Daily quota exhaustion hands back a huge retry-after (~24h). Don't silently freeze —
      // bail with a clear message. Only auto-wait for short, transient throttling.
      if (wait > 60) throw new Error(`rate limited: retry-after ${wait}s (quota exhausted?) @ ${url}`);
      console.error(`  rate limited, waiting ${wait}s...`);
      await sleep(wait * 1000);
      continue;
    }
    if (res.status === 401 && !refreshed) {
      tokenRef.value = await getToken();
      refreshed = true;
      continue;
    }
    if (!res.ok) throw new Error(`${res.status} ${res.statusText} @ ${url}`);
    return res.json();
  }
}

// Full artist objects (name, images, genres, profile URL) for a set of IDs.
// The simplified artists embedded in albums lack all of these. Tries the batch
// endpoint (50/request) first; Spotify 403s it for some dev-mode apps, so fall
// back to per-artist requests — same data, just more calls.
async function fetchArtistDetails(ids, tokenRef) {
  const details = new Map();
  const put = (a) => {
    if (!a) return;
    details.set(a.id, {
      name: a.name ?? null,
      image_url: a.images?.[0]?.url ?? null,
      genres: a.genres ?? [],
      spotify_url: a.external_urls?.spotify ?? null,
      followers: a.followers?.total ?? null,
    });
  };
  for (let i = 0; i < ids.length; i += 50) {
    const batch = ids.slice(i, i + 50);
    try {
      const data = await spFetch(`${API}/artists?ids=${batch.join(",")}`, tokenRef);
      for (const a of data.artists ?? []) put(a);
    } catch (e) {
      console.error(`  batch /artists failed (${e.message.split(" @ ")[0]}) — fetching ${batch.length} artists one by one`);
      for (const id of batch) {
        try {
          put(await spFetch(`${API}/artists/${id}`, tokenRef));
        } catch (e2) {
          console.error(`  skip ${id}: ${e2.message.split(" @ ")[0]}`);
        }
      }
    }
  }
  return details;
}

// Fetch full details for the given artist IDs and upsert name + image + genres + URL.
export async function enrichArtists(db, tokenRef, ids) {
  if (!ids.length) return;
  const details = await fetchArtistDetails(ids, tokenRef);
  for (const [id, d] of details)
    await upsertArtist(db, id, d.name, { imageUrl: d.image_url, genres: d.genres, spotifyUrl: d.spotify_url, followers: d.followers });
}

export async function fetchArtistAlbums(artistId, tokenRef) {
  const albums = new Map(); // dedup by album id (Spotify repeats across markets)
  // Spotify caps this endpoint at limit=10 for Client Credentials apps; `next` pages the rest.
  let url = `${API}/artists/${artistId}/albums?include_groups=${GROUPS}&market=${MARKET}&limit=10`;
  while (url) {
    const data = await spFetch(url, tokenRef);
    for (const al of data.items ?? []) albums.set(al.id, al);
    url = data.next; // Spotify hands back the full next-page URL
  }
  return [...albums.values()];
}

// Extra fields (image/genres/spotify_url) only overwrite when supplied — COALESCE keeps
// existing values when this call has none (e.g. the inline per-album link carries name only).
export const upsertArtist = (db, id, name, { imageUrl = null, genres = null, spotifyUrl = null, followers = null } = {}) =>
  db.query(
    `INSERT INTO artists(id,name,image_url,genres,spotify_url,followers) VALUES($1,$2,$3,$4,$5,$6)
     ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name,
       image_url=COALESCE(EXCLUDED.image_url, artists.image_url),
       genres=COALESCE(EXCLUDED.genres, artists.genres),
       spotify_url=COALESCE(EXCLUDED.spotify_url, artists.spotify_url),
       followers=COALESCE(EXCLUDED.followers, artists.followers)`,
    [id, name, imageUrl, genres, spotifyUrl, followers]
  );

const upsertAlbumRow = (db, r) =>
  db.query(
    `INSERT INTO albums(id,name,release_date,year,album_type,total_tracks,image_url,spotify_url)
     VALUES($1,$2,$3,$4,$5,$6,$7,$8)
     ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name,
       release_date=EXCLUDED.release_date, year=EXCLUDED.year, album_type=EXCLUDED.album_type,
       total_tracks=EXCLUDED.total_tracks, image_url=EXCLUDED.image_url, spotify_url=EXCLUDED.spotify_url
     WHERE albums.info_edited_at IS NULL`,
    [r.id, r.name, r.release_date, r.year, r.album_type, r.total_tracks, r.image_url, r.spotify_url]
  );

// Persist an album and ALL its credited artists: upsert the album row, upsert each
// artist by real Spotify ID, then rewrite the album_artists join rows.
export async function upsertAlbum(db, album) {
  const row = toAlbumRow(album);
  await upsertAlbumRow(db, row);
  const artists = albumArtists(album);
  await db.query("DELETE FROM album_artists WHERE album_id=$1", [row.id]);
  for (const a of artists) {
    await upsertArtist(db, a.id, a.name);
    await db.query(
      "INSERT INTO album_artists(album_id,artist_id,position) VALUES($1,$2,$3) ON CONFLICT(album_id,artist_id) DO UPDATE SET position=EXCLUDED.position",
      [row.id, a.id, a.position]
    );
  }
}

export async function openDb() {
  const db = new pg.Client({ connectionString: DATABASE_URL });
  await db.connect();
  await db.query(`
    -- aliases (연관검색어) is admin-curated via the API and never written here:
    -- upsertArtist's ON CONFLICT UPDATE doesn't touch it, so re-crawls preserve it.
    CREATE TABLE IF NOT EXISTS artists (id TEXT PRIMARY KEY, name TEXT, image_url TEXT, genres TEXT[], spotify_url TEXT, aliases TEXT[]);
    ALTER TABLE IF EXISTS artists ADD COLUMN IF NOT EXISTS image_url TEXT;
    ALTER TABLE IF EXISTS artists ADD COLUMN IF NOT EXISTS genres TEXT[];
    ALTER TABLE IF EXISTS artists ADD COLUMN IF NOT EXISTS spotify_url TEXT;
    ALTER TABLE IF EXISTS artists ADD COLUMN IF NOT EXISTS aliases TEXT[];
    ALTER TABLE IF EXISTS artists ADD COLUMN IF NOT EXISTS followers INTEGER;
    -- display_name (한글 표시 이름) is admin-curated via the API, like aliases: the
    -- upserts below never list it, so re-crawls preserve it.
    ALTER TABLE IF EXISTS artists ADD COLUMN IF NOT EXISTS display_name TEXT;
    ALTER TABLE IF EXISTS albums ADD COLUMN IF NOT EXISTS display_name TEXT;
    CREATE TABLE IF NOT EXISTS albums (
      id TEXT PRIMARY KEY, name TEXT NOT NULL,
      release_date TEXT, year INTEGER, album_type TEXT,
      total_tracks INTEGER, image_url TEXT, spotify_url TEXT
    );
    -- Many-to-many: an album can credit multiple artists, each by real Spotify ID.
    CREATE TABLE IF NOT EXISTS album_artists (
      album_id TEXT NOT NULL, artist_id TEXT NOT NULL,
      position INTEGER NOT NULL DEFAULT 0,
      PRIMARY KEY (album_id, artist_id)
    );
    CREATE INDEX IF NOT EXISTS idx_albums_year ON albums(year);
    CREATE INDEX IF NOT EXISTS idx_album_artists_artist ON album_artists(artist_id);
  `);
  return db;
}

async function main() {
  const raw = await readFile("artists.txt", "utf8").catch(() => {
    console.error("artists.txt not found. Add Spotify artist IDs/URLs, one per line.");
    process.exit(1);
  });
  const ids = [...new Set(raw.split("\n").map(parseArtistId).filter(Boolean))];
  if (!ids.length) {
    console.error("No artist IDs in artists.txt — curate your hip-hop roster first.");
    process.exit(1);
  }

  const tokenRef = { value: await getToken() };
  const db = await openDb();

  const rosterDetails = await fetchArtistDetails(ids, tokenRef);

  let total = 0;
  // Every artist that actually has albums here (roster + featured). Artists with
  // zero albums are deliberately NOT saved — no empty rows in the artists table.
  const credited = new Set();
  for (const id of ids) {
    const albums = await fetchArtistAlbums(id, tokenRef);
    if (albums.length > 0) credited.add(id);
    for (const al of albums) {
      await upsertAlbum(db, al);
      for (const a of albumArtists(al)) credited.add(a.id);
    }
    total += albums.length;
    console.log(`  ${rosterDetails.get(id)?.name ?? id}: ${albums.length} albums${albums.length ? "" : " — skipped (not saved)"}`);
  }

  // Backfill name + image for every credited artist (featured included) via the full endpoint.
  await enrichArtists(db, tokenRef, [...credited]);

  const byYear = (await db.query("SELECT year, COUNT(*)::int n FROM albums GROUP BY year ORDER BY year DESC")).rows;
  console.log(`\n${total} album rows / ${credited.size} artists → ${DATABASE_URL}`);
  console.log("by year:", byYear.map((r) => `${r.year}:${r.n}`).join("  "));
  await db.end();
}

if (process.argv[1] === fileURLToPath(import.meta.url)) main();
