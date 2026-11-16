// PathSelect.tsx is a drop-down of every folder of the app, with the root
// first. Both forms and the narrow folder row use it.

import { FIELD, SELECT } from "./ui";

/** PathSelect is a labelled select of every folder's path. */
export function PathSelect({
  label,
  paths,
  value,
  onChange,
  rootLabel = "/",
  compact = false,
}: {
  /** The select's accessible name. */
  label: string;
  /** Every folder except the root, in path order. */
  paths: string[];
  value: string;
  onChange: (path: string) => void;
  /** What the root folder is called in the list. */
  rootLabel?: string;
  /** Draw the select alone, without its label above it. */
  compact?: boolean;
}) {
  // A route can name a folder that the list does not hold yet: keep its value.
  const options = value === "/" || paths.includes(value) ? paths : [...paths, value];
  const select = (
    <select aria-label={label} className={SELECT} value={value} onChange={(event) => onChange(event.target.value)}>
      <option value="/">{rootLabel}</option>
      {options.map((path) => (
        <option key={path} value={path}>
          {path}
        </option>
      ))}
    </select>
  );
  if (compact) return select;
  return (
    <div className={FIELD}>
      <span>{label}</span>
      {select}
    </div>
  );
}
