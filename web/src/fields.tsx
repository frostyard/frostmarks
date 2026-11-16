// fields.tsx is the labelled fields that the forms share. The label's text is
// the control's accessible name.

import { FIELD, INPUT, TEXTAREA } from "./ui";

/** TextFieldProps is what a 1-line field takes. */
interface TextFieldProps {
  /** The field's accessible name, drawn above the control. */
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  /** The keyboard that a phone shows. */
  inputMode?: "url" | "text";
}

/** TextField is a labelled 1-line text input. */
export function TextField({ label, value, onChange, placeholder, inputMode }: TextFieldProps) {
  return (
    <div className={FIELD}>
      <span>{label}</span>
      <input
        type="text"
        aria-label={label}
        className={INPUT}
        value={value}
        placeholder={placeholder}
        inputMode={inputMode}
        onChange={(event) => onChange(event.target.value)}
      />
    </div>
  );
}

/** TextAreaField is a labelled field of several lines. */
export function TextAreaField({ label, value, onChange, placeholder }: Omit<TextFieldProps, "inputMode">) {
  return (
    <div className={FIELD}>
      <span>{label}</span>
      <textarea
        aria-label={label}
        className={TEXTAREA}
        value={value}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
      />
    </div>
  );
}
