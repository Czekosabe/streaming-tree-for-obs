/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Base URL of the backend REST API. Empty string means same-origin. */
  readonly VITE_API_BASE_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
