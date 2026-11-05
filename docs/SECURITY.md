# Security Policy

## Supported Versions

| Version | Supported |
| ------- | --------- |
| Latest  | ✅        |
| < 1.0   | ❌        |

## Reporting a Vulnerability

Do not report security vulnerabilities through public GitHub issues.

**Contact**: YOUR_SECURITY_EMAIL

Include:

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fixes (if any)

### Response Timeline

- **Initial Response**: Within 48 hours
- **Assessment**: Within 7 days
- **Fix**: Timeline depends on severity
- **Disclosure**: After fix is available

## Security Measures

This app uses a minimal-surface security model:

- **Permissions**: only whitelisted Go methods are exposed as bindings
- **IPC**: type-safe commands via the generated TS bindings (`wails3 generate bindings`)
- **File Access**: scoped to app directories by default (`wails/internal/core/paths.go`)
- **CSP**: Configured in `index.html`

## For Developers

### File Operations

```go
// ✅ Validate paths - prevent traversal attacks
if strings.Contains(filename, "..") {
    return errors.New("invalid filename")
}

// ❌ Never trust raw user input for paths
os.WriteFile(userInput, data, 0o600)
```

### Secrets

- Never commit secrets to version control
- Use `.env.local` (gitignored) for local secrets
- Use GitHub Secrets for CI/CD

### Dependency Audits

```bash
npm audit
govulncheck ./...   # go install golang.org/x/vuln/cmd/govulncheck@latest
```

## Resources

- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
