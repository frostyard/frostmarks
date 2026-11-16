// ConfirmSheet.tsx asks before a change that cannot be undone, and shows why
// the change failed while it stays open.

import { Sheet } from "@frostyard/lodge-ui/react";
import { useState } from "react";
import { errorText } from "./api";
import { BUTTON, PRIMARY } from "./ui";

/** ConfirmSheet is a sheet with 1 sentence and the confirm button. */
export function ConfirmSheet({
  title,
  description,
  confirmLabel,
  onConfirm,
  onClose,
}: {
  title: string;
  description: string;
  confirmLabel: string;
  /** The change: its failure's text stays in the sheet. */
  onConfirm: () => Promise<void>;
  onClose: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  /** run makes the change, and closes the sheet when it succeeds. */
  const run = async () => {
    setBusy(true);
    setError(null);
    try {
      await onConfirm();
      onClose();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Sheet title={title} role="alertdialog" onClose={onClose}>
      <p className="text-secondary">{description}</p>
      {error === null ? null : (
        <p role="alert" className="mt-2 text-primary">
          {error}
        </p>
      )}
      <div className="mt-4 flex justify-end gap-2">
        <button type="button" className={BUTTON} onClick={onClose}>
          Cancel
        </button>
        <button
          type="button"
          className={PRIMARY}
          disabled={busy}
          onClick={() => {
            void run();
          }}
        >
          {confirmLabel}
        </button>
      </div>
    </Sheet>
  );
}
