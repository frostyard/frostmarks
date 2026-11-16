// tags.ts turns the tag field of a form into the API's tag list, and the
// change between 2 lists into the tags to add and to remove.

/** parseTags splits a tag field into 1 sorted list without a repeat: commas and spaces separate tags, and each tag is lower-case. */
export function parseTags(text: string): string[] {
  const tags = new Set<string>();
  for (const part of text.split(/[\s,]+/)) {
    const tag = part.trim().toLowerCase();
    if (tag !== "") tags.add(tag);
  }
  return [...tags].sort();
}

/** tagDifference returns the tags to add and to remove to turn the old list into the new 1. */
export function tagDifference(oldTags: string[], newTags: string[]): { addTags: string[]; removeTags: string[] } {
  const before = new Set(oldTags);
  const after = new Set(newTags);
  return {
    addTags: newTags.filter((tag) => !before.has(tag)),
    removeTags: oldTags.filter((tag) => !after.has(tag)),
  };
}
