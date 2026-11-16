// BookmarkSheet.tsx adds 1 bookmark, or changes 1: its URL, title, notes,
// tags and folder. It shows the API's error text and stays open on failure.

import { Sheet } from "@frostyard/lodge-ui/react";
import { type FormEvent, useState } from "react";
import { type Bookmark, errorText, type Folder, type MarksAPI } from "./api";
import { TextAreaField, TextField } from "./fields";
import { folderPaths } from "./folders";
import { PathSelect } from "./PathSelect";
import { savableURL } from "./route";
import { parseTags, tagDifference } from "./tags";
import { BUTTON, PRIMARY } from "./ui";

/** BookmarkSheet is the form of 1 bookmark, new or stored. */
export function BookmarkSheet({
  api,
  folders,
  initial,
  folder,
  onClose,
  onSaved,
}: {
  api: MarksAPI;
  folders: Folder[];
  /** The stored bookmark, or null for a new 1. */
  initial: Bookmark | null;
  /** The folder that a new bookmark starts in. */
  folder: string;
  onClose: () => void;
  /** The change is stored: reload the view. */
  onSaved: () => void;
}) {
  const [url, setURL] = useState(initial?.url ?? "");
  const [title, setTitle] = useState(initial?.title ?? "");
  const [notes, setNotes] = useState(initial?.notes ?? "");
  const [tags, setTags] = useState(initial?.tags.join(", ") ?? "");
  const [chosen, setChosen] = useState(initial?.folder ?? folder);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  // The stored bookmark is the baseline of the next save. A call that
  // succeeds replaces it, so a failure after it keeps the change.
  const [baseline, setBaseline] = useState<Bookmark | null>(initial);
  const savable = savableURL(url.trim());

  /** save stores the bookmark: a create for a new 1, an update and a move for a stored 1. */
  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      const list = parseTags(tags);
      if (baseline === null) {
        await api.createBookmark({ url: url.trim(), title, notes, tags: list, folder: chosen });
      } else {
        const { addTags, removeTags } = tagDifference(baseline.tags, list);
        const updated = await api.updateBookmark(
          baseline.id,
          { url: url.trim(), title, notes, tags: list },
          addTags,
          removeTags,
        );
        setBaseline(updated);
        if (chosen !== baseline.folder) {
          try {
            setBaseline(await api.moveBookmark(baseline.id, chosen));
          } catch (err) {
            setError(`The change is saved, but the move to ${chosen} failed: ${errorText(err)}`);
            return;
          }
        }
      }
      onSaved();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  /** handleSubmit keeps the page, and saves the bookmark. */
  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    void save();
  };

  return (
    <Sheet
      title={initial === null ? "Add bookmark" : "Edit bookmark"}
      onClose={onClose}
      closeOnOverlay={false}
      initialFocus="first"
    >
      <form className="flex flex-col gap-3" onSubmit={handleSubmit}>
        <TextField label="URL" value={url} onChange={setURL} placeholder="https://example.com" inputMode="url" />
        {savable ? null : <p className="text-secondary">The URL must start with http:// or https://.</p>}
        <TextField label="Title" value={title} onChange={setTitle} />
        <TextAreaField label="Notes" value={notes} onChange={setNotes} />
        <TextField label="Tags" value={tags} onChange={setTags} placeholder="go, reading" />
        <PathSelect label="Folder" paths={folderPaths(folders)} value={chosen} onChange={setChosen} />
        {error === null ? null : (
          <p role="alert" className="text-primary">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-2">
          <button type="button" className={BUTTON} onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className={PRIMARY} disabled={busy || !savable}>
            Save
          </button>
        </div>
      </form>
    </Sheet>
  );
}
