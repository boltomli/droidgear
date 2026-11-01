import { i18n } from '@/i18n'

/**
 * Localisation for backend (Go) error messages.
 *
 * The Go service returns plain English strings (mirroring the Rust core).
 * Stores surface them directly in toasts and inline error states, so the
 * Wails shim translates the common, user-facing ones before rejecting.
 * Anything not matched passes through unchanged (developer-facing detail
 * stays readable).
 */
interface ErrorRule {
  /** Exact message match (after trimming). */
  exact?: string
  /** Message prefix match (for errors with dynamic parts). */
  prefix?: string
  /** i18n key under the `errors` namespace. */
  key: string
}

const RULES: ErrorRule[] = [
  // Profiles
  { exact: 'Profiles already exist', key: 'errors.profilesExist' },
  { exact: 'Invalid profile id', key: 'errors.invalidProfileId' },
  { exact: 'Invalid provider id', key: 'errors.invalidProviderId' },

  // Pi
  {
    exact: 'Pi CLI is not installed or not available in PATH',
    key: 'errors.piCliMissing',
  },
  {
    exact: 'Pi provider must define at least one model before testing',
    key: 'errors.testNeedsModel',
  },
  { exact: 'Pi provider ID cannot be empty', key: 'errors.emptyProviderId' },
  { prefix: 'Pi connection test timed out', key: 'errors.testTimeout' },

  // OMP
  { exact: 'OMP provider ID cannot be empty', key: 'errors.emptyProviderId' },
  { prefix: 'OMP auth must be', key: 'errors.ompAuthInvalid' },
  {
    exact: 'OMP declared models require a baseUrl',
    key: 'errors.ompNeedsBaseUrl',
  },
  {
    exact: 'OMP declared models require an apiKey or auth: none/oauth',
    key: 'errors.ompNeedsApiKey',
  },
  {
    exact: 'OMP model IDs must be non-empty and unique within a provider',
    key: 'errors.ompModelIds',
  },
  { prefix: "OMP model '", key: 'errors.ompModelNeedsApi' },
  { exact: 'OMP token limits must be positive', key: 'errors.ompTokenLimits' },
  { prefix: "Provider '", key: 'errors.providerNotFound' },
  { prefix: 'No models found for provider', key: 'errors.noModelsForProvider' },
  { prefix: 'No API key configured', key: 'errors.noApiKey' },
  { prefix: 'No credentials found for provider', key: 'errors.noCredentials' },

  // Model fetching / connectivity
  { prefix: 'Request failed', key: 'errors.requestFailed' },
  {
    exact: 'Base URL is required to fetch models',
    key: 'errors.baseUrlRequired',
  },
  {
    exact: 'API key is required to fetch models',
    key: 'errors.apiKeyRequired',
  },
  { exact: 'Provider returned no models', key: 'errors.noModelsReturned' },
  { prefix: 'API error', key: 'errors.apiError' },
  { prefix: 'HTTP ', key: 'errors.httpError' },
  { prefix: 'Network error', key: 'errors.networkError' },
  { prefix: 'Inference HTTP', key: 'errors.apiError' },
  { exact: 'Request timed out', key: 'errors.testTimeout' },
  { exact: 'Inference request timed out', key: 'errors.testTimeout' },
  { prefix: 'Inference network error', key: 'errors.networkError' },

  // OpenCode / Copilot
  { exact: 'Profile name is required', key: 'errors.profileNameRequired' },
  {
    exact: 'Copilot supports OpenAI and Anthropic provider types',
    key: 'errors.copilotProviderType',
  },
  {
    exact: 'Base URL must be an absolute HTTP(S) URL',
    key: 'errors.baseUrlAbsolute',
  },
  {
    exact: 'Token limits must be positive integers',
    key: 'errors.tokenLimitsPositive',
  },
  {
    exact:
      'This channel platform does not expose a supported OpenAI or Anthropic API',
    key: 'errors.channelPlatformUnsupported',
  },
  {
    exact: 'Profile id does not match its file name',
    key: 'errors.profileIdMismatch',
  },
  {
    prefix: 'Complete the Copilot BYOK profile before launching:',
    key: 'errors.copilotIncomplete',
  },
  { prefix: 'Connection failed:', key: 'errors.connectionFailed' },

  // Codex
  {
    exact: 'Cannot delete the currently active profile',
    key: 'errors.cannotDeleteActiveProfile',
  },
  { prefix: "Profile '", key: 'errors.profileNotFound' },
  {
    prefix: "Auth file missing for profile '",
    key: 'errors.authFileMissing',
  },
  {
    exact: "Provider name 'OpenAI' is reserved",
    key: 'errors.openaiReserved',
  },
  { prefix: 'Failed to read Codex CLI', key: 'errors.codexCliRead' },
  {
    prefix: 'Codex temporary run could not snapshot',
    key: 'errors.codexAuthSnapshot',
  },
  {
    exact:
      'Current auth is not an official login configuration. Only official login (codex login) credentials can be saved as auth profiles.',
    key: 'errors.codexOfficialAuthOnly',
  },

  // Claude settings files
  { exact: 'File name cannot be empty', key: 'errors.fileNameEmpty' },
  {
    exact: "Cannot use 'Global' as a custom file name",
    key: 'errors.fileNameGlobal',
  },
  {
    exact: 'File name too long (max 100 characters)',
    key: 'errors.fileNameTooLong',
  },
  {
    exact: "File name cannot start with '.' or contain '..'",
    key: 'errors.fileNameInvalid',
  },
  { prefix: 'Invalid file name', key: 'errors.fileNameInvalid' },
  {
    exact: 'Cannot delete the global settings file',
    key: 'errors.cannotDeleteGlobalSettings',
  },
  {
    exact: 'No active Claude settings file found',
    key: 'errors.noActiveClaudeSettings',
  },
  {
    exact: 'Settings file root must be a JSON object',
    key: 'errors.settingsRootObject',
  },
  { prefix: "Settings file '", key: 'errors.settingsFileProblem' },

  // Dsh
  { prefix: 'Invalid Dsh model', key: 'errors.dshInvalidInput' },
  {
    prefix: 'Invalid Dsh provider default reasoning level',
    key: 'errors.dshInvalidReasoning',
  },
  { prefix: "Dsh profile '", key: 'errors.dshProfileProblem' },
  { prefix: 'Dsh credentials file', key: 'errors.dshCredentialsLayout' },
  { prefix: "Dsh credentials ref '", key: 'errors.dshCredentialsLayout' },

  // Not-supported surfaces in this minimal build
  {
    exact: 'Native file dialogs are not available in this minimal build',
    key: 'errors.dialogsUnsupported',
  },
  {
    exact: 'Filesystem access is not available in this minimal build',
    key: 'errors.fsUnsupported',
  },
  {
    exact: 'Channels are not supported in this minimal Wails build',
    key: 'errors.channelsUnsupported',
  },

  // Channel detection / import
  { exact: 'Unable to auto-detect channel type', key: 'errors.detectFailed' },
  { exact: 'No tokens found', key: 'errors.noTokens' },
]

/**
 * Translates a backend error message when a rule matches; otherwise returns
 * the original text untouched.
 */
export function localizeWailsError(message: string): string {
  const trimmed = message.trim()
  for (const rule of RULES) {
    const matched = rule.exact
      ? trimmed === rule.exact
      : rule.prefix
        ? trimmed.startsWith(rule.prefix)
        : false
    if (!matched) continue
    const translated = i18n.t(rule.key, { defaultValue: '' })
    if (translated) return translated
  }
  return message
}
