# vspd 1.3.1

This is a patch release of vspd which includes the following changes:

- webapi: Add missed tickets to admin page ([#451](https://github.com/decred/vspd/pull/451)).

Please read the [vspd 1.3.0 release notes](https://github.com/decred/vspd/releases/tag/release-v1.3.0)
for a full list of changes since vspd 1.2.

## Dependencies

vspd 1.3.1 must be built with go 1.20 or later, and requires:

- dcrd 1.8.0
- dcrwallet 1.8.0

When deploying vspd to production, always use release versions of all binaries.
Neither vspd nor its dependencies should be built from master when handling
mainnet tickets.
