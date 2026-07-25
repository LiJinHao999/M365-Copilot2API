# Security

Do not submit tokens, cookies, HAR files, or account caches in issues or pull requests.

Please report vulnerabilities privately to the repository maintainers. Include a
minimal reproduction and affected version, but redact credentials and personal data.

The service is intended for accounts and tenants you are authorized to use. Docker
Compose binds `0.0.0.0:4141` by default so LAN clients can reach it; put TLS and
network access control in front when exposing beyond a trusted network, and keep
API keys / admin password private.
