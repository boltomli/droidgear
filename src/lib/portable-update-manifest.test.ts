import { describe, expect, it } from 'vitest'
import {
  buildPortableUpdateManifest,
  validatePortableSignatureContent,
} from '../../scripts/create-portable-update-manifest.js'

const decodedSignature = [
  'untrusted comment: signature from tauri secret key',
  'RUTestPortableSignature==',
  'trusted comment: timestamp:1775437138\tfile:droidgear_windows_x64.exe',
  'PortableTrustedComment==',
  '',
].join('\n')
const validSignature = Buffer.from(decodedSignature, 'utf8').toString('base64')

describe('portable update manifest script', () => {
  it('会将 .sig 文件正文原样写入 manifest 的 signature 字段', () => {
    const manifest = buildPortableUpdateManifest({
      version: '0.5.7',
      pubDate: '2026-04-09T00:00:00Z',
      url: 'https://example.com/droidgear_windows_x64.exe',
      signatureContent: validSignature,
      sha256: 'abc123',
      releaseUrl: 'https://example.com/releases/tag/v0.5.7',
      notes: '',
    })

    expect(manifest).toEqual({
      version: '0.5.7',
      notes: '',
      pub_date: '2026-04-09T00:00:00Z',
      url: 'https://example.com/droidgear_windows_x64.exe',
      signature: validSignature,
      sha256: 'abc123',
      release_url: 'https://example.com/releases/tag/v0.5.7',
    })
  })

  it('会拒绝混入 signer 控制台文案的伪签名内容', () => {
    const pollutedSignature = [
      'Your file was signed successfully, You can find the signature here:',
      'D:\\a\\droidgear\\droidgear_windows_x64.exe.sig',
      '',
      'Public signature:',
      'dW50cnVzdGVkIGNvbW1lbnQ6IHNpZ25hdHVyZSBmcm9tIHRhdXJpIHNlY3JldCBrZXkK',
      '',
      'Make sure to include this into the signature field of your update server.',
    ].join('\n')

    expect(() => validatePortableSignatureContent(pollutedSignature)).toThrow(
      '检测到被污染的 signer 控制台输出片段'
    )
  })

  it('会拒绝误传解码后的 minisign 文本', () => {
    expect(() => validatePortableSignatureContent(decodedSignature)).toThrow(
      '.sig 文件正文必须是单行 base64 文本'
    )
  })

  it('会拒绝解码后缺少 trusted comment 的 base64 签名正文', () => {
    expect(() =>
      validatePortableSignatureContent(
        Buffer.from(
          [
            'untrusted comment: signature from tauri secret key',
            'RUTest==',
          ].join('\n'),
          'utf8'
        ).toString('base64')
      )
    ).toThrow('解码后缺少 minisign 的 trusted comment 行')
  })
})
