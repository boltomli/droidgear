/// <reference types="vite/client" />

declare const __APP_VERSION__: string

interface ImportMetaEnv {
  readonly DROIDGEAR_DISABLE_UPDATE_CHECK?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
