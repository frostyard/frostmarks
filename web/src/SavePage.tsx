// SavePage.tsx is the page that the bookmarklet opens: the URL, its title,
// notes, tags and folder, and the button that saves them.

import type { Lodge } from "@frostyard/lodge-client";
import { type FormEvent, type MouseEvent, useCallback, useEffect, useRef, useState } from "react";
import { errorText, type MarksAPI } from "./api";
import { ErrorLine } from "./ErrorLine";
import { TextAreaField, TextField } from "./fields";
import { folderName, folderPaths } from "./folders";
import { PathSelect } from "./PathSelect";
import { LAST_FOLDER, listRoute, savableURL } from "./route";
import { parseTags } from "./tags";
import { BUTTON, PRIMARY, QUIET } from "./ui";

/** storedFolder returns the folder of the last save when it still exists, else the root. */
function storedFolder(key: string, paths: string[]): string {
  let stored: string | null = null;
  try {
    stored = globalThis.localStorage.getItem(key);
  } catch {
    stored = null;
  }
  return stored !== null && stored !== "/" && paths.includes(stored) ? stored : "/";
}

/** rememberFolder keeps the folder of this save, which the next save starts in. */
function rememberFolder(key: string, path: string): void {
  try {
    globalThis.localStorage.setItem(key, path);
  } catch {
    return;
  }
}

/** SavePageProps is what the save page takes: the route's URL and title. */
export interface SavePageProps {
  lodge: Lodge;
  api: MarksAPI;
  /** The URL of the page to save. */
  url: string;
  /** The title of the page to save. */
  title: string;
}

/** SavePage is the compact form of 1 bookmark, for a small window and a phone. */
export function SavePage({ lodge, api, url: startURL, title: startTitle }: SavePageProps) {
  const [url, setURL] = useState(startURL);
  const [title, setTitle] = useState(startTitle);
  const [notes, setNotes] = useState("");
  const [tags, setTags] = useState("");
  const [folder, setFolder] = useState("");
  const [paths, setPaths] = useState<string[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [newFolder, setNewFolder] = useState(false);
  const [newPath, setNewPath] = useState("");
  const savable = savableURL(url.trim());
  const storageKey = lodge.storageKey(LAST_FOLDER);

  // The shell names the window after the page; the route itself stays.
  const replaced = useRef(false);
  useEffect(() => {
    if (replaced.current) return;
    replaced.current = true;
    const { pathname, search } = lodge.history.location;
    lodge.history.replace(`${pathname}${search}`, "Save bookmark");
  }, [lodge]);

  // The folders, and the folder of the last save when it is still there.
  const loadFolders = useCallback(async () => {
    try {
      const list = await api.listFolders();
      const next = folderPaths(list);
      setPaths(next);
      setLoadError(null);
      setFolder((current) =>
        current === "/" || (current !== "" && next.includes(current)) ? current : storedFolder(storageKey, next),
      );
    } catch (err) {
      setLoadError(errorText(err));
    }
  }, [api, storageKey]);

  useEffect(() => {
    void loadFolders();
  }, [loadFolders]);

  /** create makes a folder, and chooses it. */
  const create = async () => {
    const path = newPath.trim();
    if (path === "") return;
    setError(null);
    try {
      await api.createFolder(path);
      setPaths((current) => (current === null ? current : [...new Set([...current, path])].sort()));
      setFolder(path);
      setNewFolder(false);
      setNewPath("");
    } catch (err) {
      setError(errorText(err));
    }
  };

  /** save stores the bookmark, and keeps the folder for the next save. */
  const save = async (event: FormEvent) => {
    event.preventDefault();
    if (!savable || folder === "" || busy) return;
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      await api.createBookmark({ url: url.trim(), title, notes, tags: parseTags(tags), folder });
      rememberFolder(storageKey, folder);
      setSaved(folder);
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  /** openFolder shows the folder of the saved bookmark in the list view. */
  const openFolder = (event: MouseEvent<HTMLAnchorElement>, path: string) => {
    event.preventDefault();
    lodge.history.push(listRoute(path), folderName(path));
  };

  /** closeWindow closes the window of the bookmarklet, when the browser allows it. */
  const closeWindow = () => {
    try {
      globalThis.top?.close();
    } catch {
      return;
    }
  };

  return (
    <div className="h-full overflow-y-auto bg-window font-sans text-primary">
      <form className="mx-auto flex w-full max-w-md flex-col gap-3 p-3" onSubmit={save}>
        <h1 className="font-semibold text-empty">Save bookmark</h1>
        {savable ? null : (
          <p role="alert" className="text-primary">
            This page cannot be saved: only http and https addresses can be.
          </p>
        )}
        <TextField label="URL" value={url} onChange={setURL} placeholder="https://example.com" inputMode="url" />
        <TextField label="Title" value={title} onChange={setTitle} />
        <TextAreaField label="Notes" value={notes} onChange={setNotes} />
        <TextField label="Tags" value={tags} onChange={setTags} placeholder="go, reading" />
        {paths === null ? (
          loadError === null ? (
            <p className="text-secondary">Loading…</p>
          ) : (
            <ErrorLine
              text={loadError}
              onRetry={() => {
                void loadFolders();
              }}
            />
          )
        ) : (
          <>
            <PathSelect label="Folder" paths={paths} value={folder} onChange={setFolder} />
            {newFolder ? (
              <div className="flex items-end gap-2">
                <div className="min-w-0 flex-1">
                  <TextField label="New folder path" value={newPath} onChange={setNewPath} placeholder="/reading" />
                </div>
                <button
                  type="button"
                  className={BUTTON}
                  onClick={() => {
                    void create();
                  }}
                >
                  Create
                </button>
                <button type="button" className={QUIET} onClick={() => setNewFolder(false)}>
                  Cancel
                </button>
              </div>
            ) : (
              <button type="button" className={QUIET} onClick={() => setNewFolder(true)}>
                New folder…
              </button>
            )}
          </>
        )}
        {error === null ? null : (
          <p role="alert" className="text-primary">
            {error}
          </p>
        )}
        {saved === null ? null : (
          <div role="status" className="flex flex-col gap-2 rounded-md bg-highlight px-2 py-1.5">
            <p>{`Saved to ${saved}.`}</p>
            <div className="flex items-center gap-2">
              <a
                href={listRoute(saved)}
                className="text-accent underline"
                onClick={(event) => openFolder(event, saved)}
              >
                Open the folder
              </a>
              <button type="button" className={BUTTON} onClick={closeWindow}>
                Close window
              </button>
            </div>
          </div>
        )}
        <div className="flex justify-end gap-2">
          <button type="submit" className={PRIMARY} disabled={busy || !savable || paths === null || folder === ""}>
            Save
          </button>
        </div>
      </form>
    </div>
  );
}
