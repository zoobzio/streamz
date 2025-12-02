# Security Policy

## Supported Versions

We release patches for security vulnerabilities. Which versions are eligible for receiving such patches depends on the CVSS v3.0 Rating:

| Version | Supported          | Status |
| ------- | ------------------ | ------ |
| latest  | :white_check_mark: | Active development |
| < latest | :x: | Security fixes only for critical issues |

## Reporting a Vulnerability

We take the security of streamz seriously. If you have discovered a security vulnerability in this project, please report it responsibly.

### How to Report

**Please DO NOT report security vulnerabilities through public GitHub issues.**

Instead, please report them via one of the following methods:

1. **GitHub Security Advisories** (Preferred)
   - Go to the [Security tab](https://github.com/zoobzio/streamz/security) of this repository
   - Click "Report a vulnerability"
   - Fill out the form with details about the vulnerability

2. **Email**
   - Send details to the repository maintainer through GitHub profile contact information
   - Use PGP encryption if possible for sensitive details

### What to Include

Please include the following information (as much as you can provide) to help us better understand the nature and scope of the possible issue:

- **Type of issue** (e.g., buffer overflow, race condition, resource exhaustion, etc.)
- **Full paths of source file(s)** related to the manifestation of the issue
- **The location of the affected source code** (tag/branch/commit or direct URL)
- **Any special configuration required** to reproduce the issue
- **Step-by-step instructions** to reproduce the issue
- **Proof-of-concept or exploit code** (if possible)
- **Impact of the issue**, including how an attacker might exploit the issue
- **Your name and affiliation** (optional)

### What to Expect

- **Acknowledgment**: We will acknowledge receipt of your vulnerability report within 48 hours
- **Initial Assessment**: Within 7 days, we will provide an initial assessment of the report
- **Resolution Timeline**: We aim to resolve critical issues within 30 days
- **Disclosure**: We will coordinate with you on the disclosure timeline

### Preferred Languages

We prefer all communications to be in English.

## Security Best Practices

When using streamz in your applications, we recommend:

1. **Keep Dependencies Updated**
   ```bash
   go get -u github.com/zoobzio/streamz
   ```

2. **Use Context Properly**
   - Always pass contexts with appropriate timeouts
   - Handle context cancellation in your processors

3. **Error Handling**
   - Never ignore errors in Result[T] values
   - Implement proper fallback mechanisms using DeadLetterQueue

4. **Input Validation**
   - Validate all inputs before processing
   - Use the Filter processor to sanitize data

5. **Resource Management**
   - Use Throttle for rate limiting
   - Set appropriate buffer sizes
   - Monitor backpressure in production

## Security Features

streamz includes several built-in security features:

- **Type Safety**: Generic types prevent type confusion attacks
- **Context Support**: Built-in cancellation and timeout support
- **Error Isolation**: Errors are properly wrapped in Result[T]
- **Resource Controls**: Throttle, Debounce, and Buffer for flow control
- **Minimal Dependencies**: Only clockz dependency reduces attack surface

## Automated Security Scanning

This project uses:

- **CodeQL**: GitHub's semantic code analysis for security vulnerabilities
- **Dependabot**: Automated dependency updates
- **golangci-lint**: Static analysis including security linters (gosec)
- **Codecov**: Coverage tracking to ensure security-critical code is tested

## Vulnerability Disclosure Policy

- Security vulnerabilities will be disclosed via GitHub Security Advisories
- We follow a 90-day disclosure timeline for non-critical issues
- Critical vulnerabilities may be disclosed sooner after patches are available
- We will credit reporters who follow responsible disclosure practices

## Credits

We thank the following individuals for responsibly disclosing security issues:

_This list is currently empty. Be the first to help improve our security!_

---

**Last Updated**: 2025-12-01
