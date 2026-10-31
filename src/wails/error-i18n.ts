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
    exact: 'Sub2API token import is not supported in this minimal Wails build',
    key: 'errors.sub2apiUnsupported',
  },
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
