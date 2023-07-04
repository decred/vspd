# vspd 1.2.1

This is a patch release of vspd which includes the following changes:

- rpc: Ignore another "duplicate tx" error ([#398](https://github.com/decred/vspd/pull/398)).

## Dependencies

vspd 1.2.1 must be built with go 1.19 or later, and requires:

- dcrd 1.8.0
- dcrwallet 1.8.0

When deploying vspd to production, always use release versions of all binaries.
Neither vspd nor its dependencies should be built from master when handling
mainnet tickets.
