// ErrorLine.tsx shows why a load failed, and the button that tries it again.

import { BUTTON } from "./ui";

/** ErrorLine is 1 line of an error's text with a Retry button. */
export function ErrorLine({ text, onRetry }: { text: string; onRetry: () => void }) {
  return (
    <div role="alert" className="flex items-center gap-2 px-3 py-2 text-secondary">
      <span className="min-w-0 flex-1">{text}</span>
      <button type="button" className={BUTTON} onClick={onRetry}>
        Retry
      </button>
    </div>
  );
}
