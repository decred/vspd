# vspd 1.1.1

This is a patch release of vspd which includes the following changes:

- Fix assignment to nil map ([#333](https://github.com/decred/vspd/pull/333)).

## Dependencies

vspd 1.1.1 must be built with go 1.16 or later, and requires:

- dcrd 1.7.1
- dcrwallet 1.7.1

When deploying vspd to production, always use release versions of all binaries.
Neither vspd nor its dependencies should be built from master when handling
mainnet tickets.
