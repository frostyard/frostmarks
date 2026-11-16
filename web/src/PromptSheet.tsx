// PromptSheet.tsx asks for 1 text, such as a folder's path, and shows why the
// call failed while it stays open.

import { Sheet } from "@frostyard/lodge-ui/react";
import { type FormEvent, useState } from "react";
import { errorText } from "./api";
import { TextField } from "./fields";
import { BUTTON, PRIMARY } from "./ui";

/** PromptSheet is a sheet with 1 text field and the submit button. */
export function PromptSheet({
  title,
  label,
  initial,
  submit,
  onSubmit,
  onClose,
}: {
  title: string;
  /** The field's accessible name. */
  label: string;
  initial: string;
  /** The submit button's text. */
  submit: string;
  /** The call: its failure's text stays in the sheet. */
  onSubmit: (text: string) => Promise<void>;
  onClose: () => void;
}) {
  const [text, setText] = useState(initial);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  /** run sends the text, and closes the sheet when the call succeeds. */
  const run = async () => {
    setBusy(true);
    setError(null);
    try {
      await onSubmit(text.trim());
      onClose();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  /** submit runs the call and keeps the page. */
  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    void run();
  };

  return (
    <Sheet title={title} onClose={onClose} closeOnOverlay={false} initialFocus="first">
      <form className="flex flex-col gap-3" onSubmit={handleSubmit}>
        <TextField label={label} value={text} onChange={setText} />
        {error === null ? null : (
          <p role="alert" className="text-primary">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-2">
          <button type="button" className={BUTTON} onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className={PRIMARY} disabled={busy || text.trim() === ""}>
            {submit}
          </button>
        </div>
      </form>
    </Sheet>
  );
}
