// FolderPanel.tsx is the folder list of the list view: a tree in a wide
// window, a select in a narrow 1, and the folder's own actions.

import { IconButton } from "@frostyard/lodge-ui/react";
import { Bookmark, FolderPlus, Pencil, Trash2 } from "lucide-react";
import type { Folder } from "./api";
import { ErrorLine } from "./ErrorLine";
import { folderPaths, folderRows } from "./folders";
import { PathSelect } from "./PathSelect";
import { QUIET, TREE_ROW } from "./ui";

/** FolderPanelProps is what the folder list takes. */
export interface FolderPanelProps {
  folders: Folder[];
  /** Why the folder list failed, when it did. */
  error: string | null;
  current: string;
  narrow: boolean;
  onSelect: (path: string) => void;
  onRetry: () => void;
  onNew: () => void;
  onRename: () => void;
  onDelete: () => void;
  onBookmarklet: () => void;
}

/** FolderPanel is the folder tree, or the narrow select, with the folder's actions. */
export function FolderPanel(props: FolderPanelProps) {
  const { folders, error, current, narrow, onSelect, onRetry, onNew, onRename, onDelete, onBookmarklet } = props;
  if (narrow) {
    return (
      <div className="flex flex-col border-separator border-b bg-toolbar">
        <div className="flex items-center gap-1 px-2 py-1">
          <PathSelect
            label="Folder"
            paths={folderPaths(folders)}
            value={current}
            onChange={onSelect}
            rootLabel="All bookmarks"
            compact
          />
          <IconButton label="New folder" icon={FolderPlus} size={24} onClick={onNew} />
          <IconButton label="Rename folder" icon={Pencil} size={24} disabled={current === "/"} onClick={onRename} />
          <IconButton label="Delete folder" icon={Trash2} size={24} disabled={current === "/"} onClick={onDelete} />
          <IconButton label="Bookmarklet" icon={Bookmark} size={24} onClick={onBookmarklet} />
        </div>
        {error === null ? null : <ErrorLine text={error} onRetry={onRetry} />}
      </div>
    );
  }
  return (
    <aside className="flex w-56 shrink-0 flex-col border-separator border-r bg-sidebar">
      <nav aria-label="Folders" className="min-h-0 flex-1 overflow-y-auto p-1">
        <ul>
          {folderRows(folders.map((folder) => folder.path)).map((row) => (
            <li key={row.path}>
              <button
                type="button"
                aria-current={row.path === current ? "page" : undefined}
                aria-label={row.path === "/" ? "All bookmarks" : row.path}
                title={row.path}
                className={TREE_ROW}
                style={{ paddingLeft: `${6 + row.depth * 12}px` }}
                onClick={() => onSelect(row.path)}
              >
                <span className="truncate">{row.name}</span>
              </button>
            </li>
          ))}
        </ul>
      </nav>
      {error === null ? null : <ErrorLine text={error} onRetry={onRetry} />}
      <div className="flex flex-col gap-0.5 border-separator border-t p-1">
        <button type="button" className={QUIET} onClick={onNew}>
          <FolderPlus size={14} aria-hidden="true" /> New folder
        </button>
        <button type="button" className={QUIET} disabled={current === "/"} onClick={onRename}>
          <Pencil size={14} aria-hidden="true" /> Rename folder
        </button>
        <button type="button" className={QUIET} disabled={current === "/"} onClick={onDelete}>
          <Trash2 size={14} aria-hidden="true" /> Delete folder
        </button>
        <button type="button" className={QUIET} onClick={onBookmarklet}>
          <Bookmark size={14} aria-hidden="true" /> Bookmarklet
        </button>
      </div>
    </aside>
  );
}
