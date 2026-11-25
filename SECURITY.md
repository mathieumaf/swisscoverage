# Security policy

swisscoverage republishes official data about mobile network sites and will
serve it through a public API. A bug here can publish wrong data under an
official source's name, or let someone ship code the maintainer did not
approve, so reports are taken seriously.

## Reporting a vulnerability

Use GitHub's private reporting form:
**[Report a vulnerability](https://github.com/mathieumaf/swisscoverage/security/advisories/new)**.
Please do not open a public issue.

Say what you observed, the steps to reproduce it, and the commit you tested.

This is a one-person project. Expect a first answer within a week. If the
report is confirmed, you are credited in the fix unless you prefer otherwise.

## Supported versions

Only the `main` branch is supported. Nothing is released or deployed yet.

## What swisscoverage promises

These are the properties a change must not break. The
[threat model](docs/security/threat-model.md) explains where each one is, or
will be, enforced.

1. **Published data says what its source says.** A site is shown only when it
   comes from a documented source, and its attributes are not guessed.
2. **Source files are data.** A downloaded file, a fixture or a contributed
   record is parsed, never executed, and its text is never placed unescaped
   into SQL, HTML or a shell command.
3. **Only reviewed code ships.** Workflows run with the least permission they
   need, third-party actions are pinned to a commit, and dependencies are
   installed from the lockfile.
4. **No secrets in the repository.** Credentials live in the deployment's
   secret store, never in code, fixtures, logs or documentation.
5. **No personal data without a stated reason.** The project does not collect
   information about the people who use it. A feature that would change this
   needs its own section in the threat model first.

## What counts

Reportable, with the highest severity first:

- A way to publish a build, an image or a deployment the maintainer did not
  approve (CI, release workflow, dependencies).
- A way to make the project store or publish data that differs from its source,
  for example through a crafted source file or contributed record.
- A way to run code, or SQL, through data: a source file, a fixture, or a
  request to the API.
- A way to read or change anything the public API does not expose, or to make
  the public instance unavailable with little effort, once it exists.

## What does not count

- Errors that are already in OFCOM's published file. Report those as a
  [data problem](https://github.com/mathieumaf/swisscoverage/issues/new/choose).
- The fact that antenna locations are public. They are published by the
  Federal Office of Communications as open government data.
- Findings that need a compromised maintainer account or machine.
