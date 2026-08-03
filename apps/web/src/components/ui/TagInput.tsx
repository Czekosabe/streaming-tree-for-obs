import { X } from 'lucide-react';
import { useState, type KeyboardEvent } from 'react';

import { cn } from '@/lib/cn';

type TagInputProps = {
  tags: string[];
  onChange: (tags: string[]) => void;
  maxTags: number;
  inputId: string;
  describedBy?: string | undefined;
  invalid?: boolean;
  placeholder?: string;
};

/**
 * Tag editor rendering each tag as its own removable chip.
 *
 * Only platforms whose capability table sets `tags: true` render this control
 * (Twitch in the current demo configuration). Validation of the resulting array
 * lives in the Zod schema, not here - this component only handles editing.
 */
export function TagInput({
  tags,
  onChange,
  maxTags,
  inputId,
  describedBy,
  invalid = false,
  placeholder = 'Type a tag and press Enter',
}: TagInputProps) {
  const [draft, setDraft] = useState('');
  const limitReached = tags.length >= maxTags;

  const commitDraft = () => {
    const value = draft.trim();
    if (value === '') return;
    if (limitReached) return;

    const isDuplicate = tags.some((tag) => tag.toLowerCase() === value.toLowerCase());
    if (isDuplicate) {
      setDraft('');
      return;
    }

    onChange([...tags, value]);
    setDraft('');
  };

  const removeTag = (index: number) => {
    onChange(tags.filter((_, i) => i !== index));
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter' || event.key === ',') {
      // Enter must not submit the surrounding form while editing tags.
      event.preventDefault();
      commitDraft();
      return;
    }
    if (event.key === 'Backspace' && draft === '' && tags.length > 0) {
      event.preventDefault();
      removeTag(tags.length - 1);
    }
  };

  return (
    <div className="space-y-2">
      <div
        className={cn(
          'flex flex-wrap items-center gap-1.5 rounded-lg border bg-surface-sunken p-2',
          'transition-colors duration-150 focus-within:border-accent',
          invalid ? 'border-status-error/70' : 'border-line',
        )}
      >
        <ul className="contents">
          {tags.map((tag, index) => (
            <li key={tag}>
              <span
                className={cn(
                  'inline-flex items-center gap-1 rounded-md border border-accent/35 bg-accent/12',
                  'py-0.5 pr-0.5 pl-2 text-xs font-medium text-accent-soft',
                )}
              >
                {tag}
                <button
                  type="button"
                  onClick={() => removeTag(index)}
                  aria-label={`Remove tag ${tag}`}
                  className={cn(
                    'inline-flex size-5 items-center justify-center rounded',
                    'text-accent-soft/70 transition-colors hover:bg-accent/25 hover:text-ink',
                  )}
                >
                  <X aria-hidden="true" className="size-3" />
                </button>
              </span>
            </li>
          ))}
        </ul>

        <input
          id={inputId}
          type="text"
          value={draft}
          disabled={limitReached}
          aria-invalid={invalid}
          aria-describedby={describedBy}
          onChange={(event) => setDraft(event.target.value)}
          onKeyDown={handleKeyDown}
          onBlur={commitDraft}
          placeholder={limitReached ? `Limit of ${maxTags} tags reached` : placeholder}
          className={cn(
            'min-w-[10rem] flex-1 bg-transparent px-1 py-0.5 text-sm text-ink outline-none',
            'placeholder:text-ink-faint disabled:cursor-not-allowed',
          )}
        />
      </div>

      <p className="text-[11px] text-ink-faint">
        {tags.length} / {maxTags} tags. Press Enter or comma to add, Backspace to remove the last
        one.
      </p>
    </div>
  );
}
