import React, { useEffect, useState, useRef } from "react";
import { createRoot } from "react-dom/client";
import {
  BrowserRouter,
  NavLink,
  Navigate,
  Route,
  Routes,
  useLocation,
  useNavigate,
} from "react-router-dom";
import "./style.css";
import { Icon } from "./icons";
import {
  StatusBadge,
  StatusSection,
  ActiveProgress,
  finished,
  blocked,
} from "./status";
import { DeletionDialog } from "./deletion-dialog";
import { DeletionKeys } from "./deletions";
import { DownloadSettings } from "./downloads";
import type { TransferProgress } from "./api";
import { api, setCSRF } from "./api";
import { Search, Library, Connections } from "./music";
import { Imports, NewImport, MigrationPage } from "./imports";
type User = {
  id: string;
  username: string;
  role: string;
  direct: boolean;
  directDelete: boolean;
  quality: string[];
};
type Request = {
  transfer?: TransferProgress;
  state: string;
  kind: string;
  detail?: string;
  trackCount?: number;
  items?: {
    title: string;
    artist: string;
    state: string;
    detail: string;
    transfer?: TransferProgress;
  }[];
  id: string;
  owner: string;
  title: string;
  artist: string;
  target: string;
  decision: string;
  reason: string;
  upgrade: boolean;
  quality: string[];
};
function App() {
  const [setup, setSetup] = useState(false);
  const [user, setUser] = useState<User | null>(null),
    [loading, setLoading] = useState(true),
    [error, setError] = useState("");
  const [collapsed, setCollapsed] = useState(
    localStorage.getItem("sidebar") === "collapsed",
  );
  const location = useLocation();
  const navigate = useNavigate();
  const [sidebarQuery, setSidebarQuery] = useState("");
  const searchInput = useRef<HTMLInputElement>(null);
  async function load() {
    try {
      const first = await api<{ required: boolean }>("/setup");
      setSetup(first.required);
      if (first.required) {
        setUser(null);
        return;
      }
      const s = await api<{ user: User; csrf: string }>("/session");
      setCSRF(s.csrf);
      setUser(s.user);
    } catch {
      setUser(null);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    void load();
  }, []);
  useEffect(() => {
    setError("");
    document.querySelector<HTMLElement>("main h1")?.focus();
  }, [location.pathname]);
  async function login(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    try {
      if (setup) await api("/setup", "POST", Object.fromEntries(f));
      const s = await api<{ csrf: string }>(
        "/session",
        "POST",
        Object.fromEntries(f),
      );
      setCSRF(s.csrf);
      await load();
    } catch (e) {
      setError(String((e as Error).message));
    }
  }
  if (loading)
    return (
      <div className="login">
        <p>Opening your workspace…</p>
      </div>
    );
  if (!user)
    return (
      <div className="login">
        <div className="brand">◉ ZenSync</div>
        <h1>{setup ? "Welcome to ZenSync" : "Your music, together."}</h1>
        <p>
          {setup
            ? "Create the administrator account to start migrating your music."
            : "Sign in to your shared music workspace."}
        </p>
        <form onSubmit={login}>
          <label>
            Username
            <input name="username" autoComplete="username" required />
          </label>
          <label>
            Password
            <input
              name="password"
              type="password"
              autoComplete={setup ? "new-password" : "current-password"}
              minLength={setup ? 12 : undefined}
              maxLength={72}
              required
            />
          </label>
          {error && <p role="alert">{error}</p>}
          <button>{setup ? "Create administrator" : "Sign in"}</button>
        </form>
      </div>
    );
  const nav = [
    ["/library/songs", "Library"],
    ["/search", "Search"],
    ["/imports", "Imports"],
    ["/requests", "Requests"],
    ["/activity", "Activity"],
    ["/settings/connections", "Settings"],
  ];
  return (
    <div className={"shell " + (collapsed ? "collapsed" : "")}>
      <aside>
        <div className="sidebar-top">
          <div className="brand">
            <span aria-hidden="true">◉</span>
            <span className="navtext">ZenSync</span>
          </div>
          <button
            className="quiet collapse"
            title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            aria-expanded={!collapsed}
            onClick={() => {
              setCollapsed(!collapsed);
              localStorage.setItem(
                "sidebar",
                collapsed ? "expanded" : "collapsed",
              );
            }}
          >
            <Icon name={collapsed ? "expand" : "collapse"} />
          </button>
        </div>
        {collapsed ? (
          <button
            className="sidebar-search-toggle quiet"
            aria-label="Open sidebar search"
            title="Search music"
            onClick={() => {
              setCollapsed(false);
              localStorage.setItem("sidebar", "expanded");
              requestAnimationFrame(() => searchInput.current?.focus());
            }}
          >
            <Icon name="search" />
          </button>
        ) : (
          <form
            className="sidebar-search"
            role="search"
            onSubmit={(e) => {
              e.preventDefault();
              if (sidebarQuery.trim()) {
                navigate(
                  "/search?q=" + encodeURIComponent(sidebarQuery.trim()),
                );
                setSidebarQuery("");
              }
            }}
          >
            <Icon name="search" />
            <input
              ref={searchInput}
              aria-label="Search music"
              placeholder="Search music…"
              value={sidebarQuery}
              onChange={(e) => setSidebarQuery(e.target.value)}
              maxLength={500}
            />
          </form>
        )}
        <nav aria-label="Main navigation">
          {nav.map(([to, label], i) => (
            <NavLink to={to} key={to} title={label} aria-label={label}>
              <span className="symbol">
                <Icon
                  name={
                    [
                      "library",
                      "search",
                      "imports",
                      "requests",
                      "activity",
                      "settings",
                    ][i]
                  }
                />
              </span>
              <span className="navtext">{label}</span>
            </NavLink>
          ))}
        </nav>
        <div className="account">
          <strong>{user.username}</strong>
          <small>{user.role}</small>
          <button
            className="quiet signout"
            aria-label="Sign out"
            title="Sign out"
            onClick={async () => {
              try {
                await api("/session", "DELETE");
                setUser(null);
              } catch (e) {
                setError((e as Error).message);
              }
            }}
          >
            <Icon name="logout" />
            <span className="navtext">Sign out</span>
          </button>
        </div>
      </aside>
      <main>
        <header>
          <span>Shared music workspace</span>
          <span className="pill">Music migration</span>
        </header>
        {error && <p role="alert">{error}</p>}
        <Routes>
          <Route path="/requests" element={<Requests user={user} />} />
          <Route
            path="/activity"
            element={<Activity admin={user.role === "admin"} />}
          />
          <Route
            path="/settings/*"
            element={<Settings user={user} reload={load} />}
          />
          <Route
            path="/library/:view"
            element={
              <Library canDelete={user.role === "admin" || user.directDelete} />
            }
          />
          <Route
            path="/search"
            element={
              <Search canDelete={user.role === "admin" || user.directDelete} />
            }
          />
          <Route path="/imports" element={<Imports />} />
          <Route path="/imports/new" element={<NewImport />} />
          <Route path="/imports/:id" element={<MigrationPage />} />
          <Route
            path="/imports/:id/songs/:sourceId"
            element={<MigrationPage />}
          />
          <Route
            path="/imports/:id/playlists/:playlistId"
            element={<MigrationPage />}
          />
          <Route path="*" element={<Navigate to="/imports" replace />} />
        </Routes>
      </main>
    </div>
  );
}
function Requests({ user }: { user: User }) {
  const [deletionToApprove, setDeletionToApprove] = useState<Request | null>(
    null,
  );
  const [rows, setRows] = useState<Request[]>([]),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [loaded, setLoaded] = useState(false);
  async function load() {
    try {
      setRows(await api<Request[]>("/requests"));
      setLoaded(true);
    } catch (e) {
      setError((e as Error).message);
    }
  }
  useEffect(() => {
    void load();
    const timer = setInterval(() => void load(), 4000);
    return () => clearInterval(timer);
  }, []);
  async function decide(r: Request, decision: string, reason = "") {
    setBusy(true);
    setError("");
    try {
      await api("/requests/" + r.id, "POST", { decision, reason });
      await load();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <h1 tabIndex={-1}>
        Requests <span className="count">{rows.length}</span>
      </h1>
      <p>
        {user.role === "admin"
          ? "Review acquisition and deletion requests across the workspace."
          : "Your music requests and approval decisions."}
      </p>
      <div className="actions toolbar">
        <NavLink className="button" to="/search">
          Search or paste a Deezer link
        </NavLink>
        <NavLink to="/imports">Request from a migration</NavLink>
      </div>
      {error && <p role="alert">{error}</p>}
      {!loaded ? (
        <p>Loading requests…</p>
      ) : rows.length === 0 ? (
        <p className="emptytext">
          No requests yet. Find a song in Search or select missing songs from a
          migration.
        </p>
      ) : (
        ["In progress", "Requested", "Completed"].map((title) => {
          const group = rows.filter(
            (r) =>
              title ===
              (r.decision === "awaiting_approval"
                ? "Requested"
                : finished(r.state || r.decision)
                  ? "Completed"
                  : "In progress"),
          );
          return (
            <StatusSection key={title} title={title} count={group.length}>
              {group.map((r) => (
                <article className="request" key={r.id}>
                  <div className="row">
                    <div>
                      <strong>
                        {r.title}
                        {r.kind === "delete"
                          ? " (Delete song)"
                          : r.kind === "album"
                            ? " (Album)"
                            : ""}
                      </strong>
                      <small>
                        {r.artist || "Artist not specified"} / {r.target}
                        {r.upgrade ? " / Upgrade" : ""}
                      </small>
                    </div>
                    <StatusBadge state={r.state || r.decision} />
                  </div>
                  <ActiveProgress
                    state={r.state}
                    title={r.title}
                    transfer={r.transfer}
                  />
                  {r.kind === "album" && (
                    <details>
                      <summary>
                        {r.trackCount} tracks ·{" "}
                        {r.items?.filter((i) => i.state === "available")
                          .length || 0}{" "}
                        available
                      </summary>
                      <p className="muted">
                        One approval covers this saved recording list and
                        quality preference.
                      </p>
                      {r.items?.map((i, index) => (
                        <div key={index}>
                          <div className="row">
                            <div>
                              <strong>
                                {index + 1}. {i.title}
                              </strong>
                              <small>
                                {i.artist}
                                {i.detail ? " / " + i.detail : ""}
                              </small>
                            </div>
                            <StatusBadge state={i.state} />
                          </div>
                          <ActiveProgress
                            state={i.state}
                            title={i.title}
                            transfer={i.transfer}
                          />
                        </div>
                      ))}
                    </details>
                  )}
                  {r.kind === "delete" && (
                    <p className="muted">
                      Removes this song for everyone in the shared library.{" "}
                      {r.state &&
                        `${r.state.replaceAll("_", " ")}: ${r.detail || ""}`}
                    </p>
                  )}
                  {r.reason && <p className="muted">{r.reason}</p>}
                  {r.decision === "awaiting_approval" && (
                    <div className="actions">
                      {user.role === "admin" && (
                        <>
                          <button
                            disabled={busy}
                            onClick={() => {
                              if (r.kind === "delete") setDeletionToApprove(r);
                              else void decide(r, "approved");
                            }}
                          >
                            Approve
                          </button>
                          <form
                            className="decline"
                            onSubmit={(e) => {
                              e.preventDefault();
                              void decide(
                                r,
                                "declined",
                                String(
                                  new FormData(e.currentTarget).get("reason"),
                                ),
                              );
                            }}
                          >
                            <input
                              name="reason"
                              aria-label={"Reason for declining " + r.title}
                              placeholder="Reason for declining"
                              maxLength={1000}
                              required
                            />
                            <button className="quiet" disabled={busy}>
                              Decline
                            </button>
                          </form>
                        </>
                      )}
                      {r.owner === user.id && (
                        <button
                          className="quiet"
                          disabled={busy}
                          onClick={() => void decide(r, "withdrawn")}
                        >
                          Withdraw
                        </button>
                      )}
                    </div>
                  )}
                </article>
              ))}
            </StatusSection>
          );
        })
      )}
      <DeletionDialog
        open={deletionToApprove !== null}
        mode="approve"
        song={deletionToApprove || { title: "", artist: "" }}
        onClose={() => setDeletionToApprove(null)}
        onConfirm={async () => {
          if (!deletionToApprove) return;
          await api("/requests/" + deletionToApprove.id, "POST", {
            decision: "approved",
            reason: "",
          });
          await load();
        }}
      />
    </>
  );
}
function Activity({ admin }: { admin: boolean }) {
  const [rows, setRows] = useState<
      {
        id: string;
        title: string;
        state: string;
        stage: string;
        transfer?: TransferProgress;
      }[]
    >([]),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true);
  async function load() {
    try {
      setRows(await api<typeof rows>("/activity"));
      setLoading(false);
    } catch (e) {
      setError((e as Error).message);
    }
  }
  useEffect(() => {
    void load();
    const timer = setInterval(() => void load(), 3000);
    return () => clearInterval(timer);
  }, []);
  async function retry(id: string) {
    try {
      await api("/tasks/" + id, "POST");
      await load();
    } catch (e) {
      setError((e as Error).message);
    }
  }
  return (
    <>
      <h1 tabIndex={-1}>Activity</h1>
      <p>Follow downloads, library scans and migration tasks.</p>
      {admin && (
        <NavLink to="/settings/downloads">Manage parallel downloads</NavLink>
      )}
      {error && <p role="alert">{error}</p>}
      {loading ? (
        <p>Loading activity…</p>
      ) : rows.length === 0 ? (
        <p className="emptytext">
          No execution tasks. Requests awaiting approval do not occupy a
          transfer slot.
        </p>
      ) : (
        ["In progress", "Blocked", "Completed"].map((title) => {
          const group = rows.filter(
            (r) =>
              title ===
              (finished(r.state)
                ? "Completed"
                : blocked(r.state)
                  ? "Blocked"
                  : "In progress"),
          );
          return (
            <StatusSection key={title} title={title} count={group.length}>
              {group.map((r) => (
                <div className="activity-item" key={r.id}>
                  <div className="row">
                    <div>
                      <strong>{r.title}</strong>
                      <small>{r.stage}</small>
                    </div>
                    <div className="actions">
                      <StatusBadge state={r.state} />
                      {(r.state === "failed" || r.state === "blocked") && (
                        <button
                          className="quiet"
                          onClick={() => void retry(r.id)}
                        >
                          Retry
                        </button>
                      )}
                    </div>
                  </div>
                  <ActiveProgress
                    state={r.state}
                    title={r.title}
                    transfer={r.transfer}
                  />
                </div>
              ))}
            </StatusSection>
          );
        })
      )}
    </>
  );
}
function Settings({
  user,
  reload,
}: {
  user: User;
  reload: () => Promise<void>;
}) {
  const [users, setUsers] = useState<User[]>([]),
    [message, setMessage] = useState(""),
    [busy, setBusy] = useState(false);
  const location = useLocation();
  async function loadUsers() {
    if (user.role === "admin") setUsers(await api<User[]>("/users"));
  }
  useEffect(() => {
    loadUsers().catch((e) => setMessage(e.message));
  }, []);
  async function add(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget,
      f = new FormData(form);
    setBusy(true);
    try {
      await api("/users", "POST", {
        username: f.get("username"),
        password: f.get("password"),
        direct: f.get("direct") === "on",
        directDelete: f.get("directDelete") === "on",
      });
      form.reset();
      await loadUsers();
      setMessage("User created.");
    } catch (e) {
      setMessage((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <h1 tabIndex={-1}>Settings</h1>
      <nav className="tabs" aria-label="Settings">
        <NavLink to="/settings/connections">Connections</NavLink>
        <NavLink to="/settings/quality">Quality</NavLink>
        <NavLink to="/settings/api">API keys</NavLink>
        {user.role === "admin" && (
          <NavLink to="/settings/downloads">Downloads</NavLink>
        )}
        {user.role === "admin" && <NavLink to="/settings/users">Users</NavLink>}
      </nav>
      {message && <p role="status">{message}</p>}
      {(location.pathname.endsWith("/users") ||
        location.pathname.endsWith("/people")) &&
      user.role === "admin" ? (
        <>
          <h2>Users & permissions</h2>
          {users.map((u) => (
            <div className="row" key={u.id}>
              <strong>
                {u.username}
                <small>{u.role}</small>
              </strong>
              <div>
                {u.role === "admin" ? (
                  <span>Administrator</span>
                ) : (
                  <label className="check permission-toggle">
                    <input
                      type="checkbox"
                      checked={u.direct}
                      disabled={busy}
                      onChange={async (e) => {
                        const direct = e.target.checked;
                        setUsers((users) =>
                          users.map((current) =>
                            current.id === u.id
                              ? { ...current, direct }
                              : current,
                          ),
                        );
                        setBusy(true);
                        try {
                          await api("/users/" + u.id, "PUT", { direct });
                          await loadUsers();
                          setMessage(
                            "Permissions saved for " + u.username + ".",
                          );
                        } catch (e) {
                          setUsers((users) =>
                            users.map((current) =>
                              current.id === u.id ? u : current,
                            ),
                          );
                          setMessage((e as Error).message);
                        } finally {
                          setBusy(false);
                        }
                      }}
                    />
                    Allow adding music without approval
                  </label>
                )}
                {u.role !== "admin" && (
                  <label className="check">
                    <input
                      type="checkbox"
                      checked={u.directDelete}
                      disabled={busy}
                      onChange={async (e) => {
                        const directDelete = e.target.checked;
                        setUsers((current) =>
                          current.map((person) =>
                            person.id === u.id
                              ? { ...person, directDelete }
                              : person,
                          ),
                        );
                        setBusy(true);
                        try {
                          await api(`/users/${u.id}/permissions`, "PUT", {
                            directDelete,
                          });
                          await loadUsers();
                          if (u.id === user.id) await reload();
                          setMessage("Deletion permission updated.");
                        } catch (err) {
                          setUsers((current) =>
                            current.map((person) =>
                              person.id === u.id
                                ? { ...person, directDelete: !directDelete }
                                : person,
                            ),
                          );
                          setMessage((err as Error).message);
                        } finally {
                          setBusy(false);
                        }
                      }}
                    />
                    Allow deleting music without approval
                  </label>
                )}
              </div>
            </div>
          ))}
          <h2>Add a user</h2>
          <form onSubmit={add}>
            <div className="fields">
              <label>
                Username
                <input
                  name="username"
                  autoComplete="off"
                  maxLength={80}
                  required
                />
              </label>
              <label>
                Initial password
                <input
                  name="password"
                  type="password"
                  autoComplete="new-password"
                  minLength={12}
                  maxLength={72}
                  required
                />
              </label>
            </div>
            <label className="check">
              <input name="direct" type="checkbox" /> Allow adding music without
              approval
            </label>
            <label className="check">
              <input name="directDelete" type="checkbox" /> Allow deleting music
              without approval
            </label>
            <p className="muted">
              Adding and deleting have separate permissions. Other deletion
              requests require admin approval. Upgrades still require admin
              approval.
            </p>
            <button disabled={busy}>Create user</button>
          </form>
        </>
      ) : location.pathname.endsWith("/api") ? (
        <DeletionKeys />
      ) : location.pathname.endsWith("/downloads") && user.role === "admin" ? (
        <DownloadSettings />
      ) : location.pathname.endsWith("/quality") ? (
        <>
          <h2>Your quality preference</h2>
          <p>Requests preserve a snapshot of this preference when submitted.</p>
          <form
            onSubmit={async (e) => {
              e.preventDefault();
              setBusy(true);
              try {
                await api("/quality", "PUT", {
                  quality: String(
                    new FormData(e.currentTarget).get("quality"),
                  ).split(","),
                });
                await reload();
                setMessage("Quality preference saved.");
              } catch (e) {
                setMessage((e as Error).message);
              } finally {
                setBusy(false);
              }
            }}
          >
            <label>
              Allowed qualities, in order
              <select name="quality" defaultValue={user.quality.join(",")}>
                <option value="lossless,320,128">Lossless → 320 → 128</option>
                <option value="lossless">Lossless only</option>
                <option value="320,128">320 → 128</option>
                <option value="320">320 only</option>
                <option value="128">128 only</option>
              </select>
            </label>
            <button disabled={busy}>Save preference</button>
          </form>
          <p className="muted">
            d-fi tries only the allowed qualities in this order. Existing
            playable music is reused; replacing shared audio is not enabled yet.
          </p>
        </>
      ) : (
        <Connections admin={user.role === "admin"} />
      )}
    </>
  );
}
createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </React.StrictMode>,
);
