// Extends Vitest's `expect` with React Testing Library's jest-dom matchers
// (toBeInTheDocument, toHaveTextContent, ...) and registers automatic
// unmount/cleanup after each test, so rendered-component tests never leak
// DOM nodes into the next test.
import '@testing-library/jest-dom/vitest';

import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

afterEach(() => {
  cleanup();
});
