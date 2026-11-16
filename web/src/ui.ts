// ui.ts holds the class names that the UI's own controls share. Every colour
// comes from @frostyard/lodge-ui's theme, and every height from its tokens.

/** BUTTON is a text button that runs 1 command. */
export const BUTTON =
  "inline-flex h-[var(--control-h)] shrink-0 items-center justify-center gap-1 rounded-md border border-separator px-2.5 text-primary hover:bg-selection-inactive disabled:opacity-40";

/** PRIMARY is the text button of a form's main action. */
export const PRIMARY =
  "inline-flex h-[var(--control-h)] shrink-0 items-center justify-center gap-1 rounded-md bg-accent px-3 font-semibold text-accent-contrast hover:opacity-90 disabled:opacity-40";

/** QUIET is a text button with no frame, for a panel's foot. */
export const QUIET =
  "inline-flex h-[var(--control-h)] items-center gap-1 rounded-md px-2 text-secondary hover:bg-selection-inactive hover:text-primary disabled:opacity-40";

/** INPUT is a 1-line field. */
export const INPUT =
  "h-[var(--control-h)] w-full min-w-0 rounded-md border border-separator bg-window px-2 text-primary placeholder:text-tertiary";

/** TEXTAREA is a field of several lines. */
export const TEXTAREA =
  "min-h-16 w-full resize-y rounded-md border border-separator bg-window px-2 py-1 text-primary placeholder:text-tertiary";

/** SELECT is a drop-down of 1 choice. */
export const SELECT = "h-[var(--control-h)] min-w-0 rounded-md border border-separator bg-window px-2 text-primary";

/** FIELD is the column of a label and its control. */
export const FIELD = "flex min-w-0 flex-col gap-1 text-secondary";

/** TREE_ROW is the row of a folder in the tree. */
export const TREE_ROW =
  "flex h-[var(--row-h)] w-full items-center gap-1.5 rounded-md px-1.5 text-left text-primary text-sidebar-row hover:bg-selection-inactive";
