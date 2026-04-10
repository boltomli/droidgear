export interface PortableUpdateManifest {
  version: string
  notes: string
  pub_date: string
  url: string
  signature: string
  sha256: string
  release_url: string
}

export interface BuildPortableUpdateManifestInput {
  version: string
  pubDate: string
  url: string
  signatureContent: string
  sha256: string
  releaseUrl: string
  notes?: string
}

export function validatePortableSignatureContent(
  signatureContent: string
): string
export function encodePortableSignature(signatureContent: string): string
export function buildPortableUpdateManifest(
  input: BuildPortableUpdateManifestInput
): PortableUpdateManifest
export function readPortableSignatureFile(signatureFilePath: string): string
export function writePortableManifest(
  outputPath: string,
  manifest: PortableUpdateManifest
): void
