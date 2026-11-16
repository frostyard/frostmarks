// BookmarkletSheet.tsx explains the browser bookmarklet, and offers it as a
// link to drag and as text to copy.

import { Sheet } from "@frostyard/lodge-ui/react";
import { useEffect, useRef } from "react";
import { bookmarklet } from "./bookmarklet";
import { TEXTAREA } from "./ui";

/** BookmarkletSheet is the text of the bookmarklet and the link that drags it. */
export function BookmarkletSheet({ onClose }: { onClose: () => void }) {
  const code = bookmarklet(globalThis.location.origin);
  const link = useRef<HTMLAnchorElement>(null);

  // React drops a javascript: URL from href, so the attribute is set here.
  useEffect(() => {
    link.current?.setAttribute("href", code);
  }, [code]);

  return (
    <Sheet title="Save from your browser" onClose={onClose}>
      <p className="text-secondary">
        Drag the link below to your browser's bookmarks bar, and press it on any page to save that page.
      </p>
      <p className="mt-3">
        {/* The bookmarklet's link is dragged, never clicked: its javascript: URL is set below, because React drops that scheme from href. */}
        {/* biome-ignore lint/a11y/noStaticElementInteractions lint/a11y/useKeyWithClickEvents: a bookmarklet is a javascript: URL to drag, and a click in the page must do nothing */}
        <a
          ref={link}
          draggable
          className="inline-flex h-[var(--control-h)] items-center rounded-md bg-accent px-3 font-semibold text-accent-contrast"
          // biome-ignore lint/a11y/useValidAnchor: a bookmarklet is a javascript: URL to drag, and a click in the page must do nothing
          onClick={(event) => event.preventDefault()}
        >
          Save to Bookmarks
        </a>
      </p>
      <label className="mt-4 flex flex-col gap-1 text-secondary">
        <span>Bookmarklet code</span>
        <textarea
          aria-label="Bookmarklet code"
          className={`${TEXTAREA} font-mono`}
          readOnly
          rows={3}
          value={code}
          onFocus={(event) => event.target.select()}
        />
      </label>
    </Sheet>
  );
}
