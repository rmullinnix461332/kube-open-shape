# Security Policy

## Project Status

Kube Open Shape is under active development and should currently be considered alpha software.

The Janitor operates in observe-only mode. Neutralize and Delete actions are not currently enabled. APIs, configuration schemas, command behavior, storage formats, and security boundaries may change before a stable release.

Do not deploy Kube Open Shape with broader Kubernetes permissions than necessary.

## Supported Versions

Until stable releases are available, security fixes are applied only to the latest code on the default branch.

| Version | Supported |
|---|---|
| Default branch | Yes |
| Latest tagged prerelease | Best effort |
| Older commits and prereleases | No |

After stable releases begin, this section will be updated with a formal support policy.

## Reporting a Vulnerability

Do not report suspected security vulnerabilities through a public GitHub issue, discussion, or pull request.

Use GitHub Private Vulnerability Reporting:

[Report a vulnerability privately](https://github.com/rmullinnix461332/kube-open-shape/security/advisories/new)

Include as much of the following information as possible:

- Description of the vulnerability.
- Affected commit, tag, or version.
- Kubernetes version and distribution.
- Kube Open Shape deployment mode.
- Required Kubernetes permissions.
- Reproduction steps or proof of concept.
- Potential impact.
- Whether exploitation requires authenticated cluster access.
- Suggested mitigation, if known.
- Logs or output with credentials and sensitive cluster information removed.

If GitHub Private Vulnerability Reporting is unavailable, open a public issue containing no vulnerability details and request a private reporting channel.

## Response Process

Security reports are handled on a best-effort basis.

The expected process is:

1. Acknowledge receipt of the report.
2. Reproduce and assess the reported behavior.
3. Determine affected versions and configurations.
4. Develop and test a correction or mitigation.
5. Coordinate disclosure with the reporter.
6. Publish a security advisory and corrected release when appropriate.

Please allow reasonable time for investigation and remediation before public disclosure.

## Security-Relevant Areas

Reports are particularly valuable when they involve:

- Unauthorized Kubernetes resource access.
- Kubernetes RBAC privilege escalation.
- Exposure of Secret values or sensitive resource content.
- Local API authentication or authorization bypass.
- Unsafe network exposure of the local API.
- Execution outside the scope authorized by a Janitor rule.
- Bypass of Janitor protection or approval constraints.
- Incorrect detection of an active reconciliation authority that enables mutation.
- Execution-plan tampering or approval-digest bypass.
- Unsafe handling of graph dependencies, shared resources, or persistent data.
- Path traversal, file overwrite, or unsafe SQLite access.
- Malicious ShapeDefinition, RelationshipDefinition, or JanitorRule processing.
- Denial of service caused by crafted Kubernetes resources.
- Supply-chain compromise in distributed binaries, images, or dependencies.

## Security Model

Kube Open Shape observes Kubernetes resources and derives operational knowledge from them.

Depending on its RBAC permissions and configuration, the edge may be able to observe sensitive Kubernetes resource content. Treat the following as potentially sensitive:

- SQLite databases.
- Knowledge-graph exports.
- JSON and YAML command output.
- API responses.
- Diagnostic logs.
- Finding evidence.
- Resource metadata and annotations.
- Secret objects available to the configured service account.

Do not expose the local API outside a trusted network boundary unless it is protected by an appropriate authenticated proxy, network policy, or equivalent control.

Run Kube Open Shape using a dedicated service account with least-privilege RBAC. Avoid granting mutation permissions while using observe-only functionality.

## Janitor Safety

The current Janitor implementation produces findings but does not execute Neutralize or Delete actions.

Future mutation capabilities are designed around these principles:

- Uncertainty results in inaction.
- Active reconcilers protect managed resources from mutation.
- Candidate affinity and provisional classifications cannot authorize action.
- Destructive actions require a complete graph-derived execution plan.
- Operator approval applies to an immutable plan digest.
- Material state changes invalidate prior approval.
- Persistent and shared resources require explicit disposition.
- Subsystem degradation remains visible and blocks mutation.
- Every action must be traceable and independently verifiable.

A failure of the Janitor safety model should be reported as a security vulnerability when it could permit unauthorized or unsafe cluster mutation.

## Deployment Recommendations

Until the project reaches a stable release:

- Use a disposable or non-production cluster for evaluation.
- Use read-only Kubernetes permissions whenever possible.
- Restrict access to the local API.
- Protect SQLite data and graph exports.
- Review generated ShapeDefinitions and classifications before applying them.
- Do not treat candidate affinity as authoritative knowledge.
- Review ownership and relationship evidence before relying on findings.
- Pin container images and binaries to an explicit commit or release.
- Review RBAC manifests before installation.
- Do not assume alpha interfaces are backward compatible.

## Out of Scope

The following are generally not considered Kube Open Shape vulnerabilities unless they create a direct security impact within the project:

- Vulnerabilities in Kubernetes itself.
- Vulnerabilities in an unrelated third-party controller or release manager.
- Incorrect business or organizational classifications without a security consequence.
- Missing product features documented as planned.
- Expected behavior requiring already-authorized cluster administrator access.
- Findings caused solely by inaccurate or incomplete user-supplied metadata.
- Unsupported historical commits or prereleases.
- Social-engineering attacks against project maintainers or users.

Third-party dependency vulnerabilities may still be reported when they are reachable or exploitable through Kube Open Shape.

## Sensitive Information

Do not include the following in public issues or reports:

- Kubernetes bearer tokens.
- Kubeconfig files.
- Secret values.
- Private repository URLs containing credentials.
- Cloud account credentials.
- Internal cluster endpoints.
- Personally identifiable information.
- Complete unredacted knowledge-graph exports from private clusters.

Redact sensitive information while preserving enough technical detail to reproduce the issue.

## Coordinated Disclosure

Reporters are asked to avoid public disclosure until:

- The vulnerability has been investigated.
- A mitigation or correction is available, when practical.
- A coordinated disclosure date has been agreed upon.

Kube Open Shape will credit reporters in published advisories unless anonymity is requested.
