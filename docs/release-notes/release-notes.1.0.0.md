# vspd 1.0.0

This is the initial release of vspd which is intended to replace
[dcrstakepool](https://github.com/decred/dcrstakepool) as the reference
implementation of a Decred Voting Service Provider.

The [Decred blog](https://blog.decred.org/2020/06/02/A-More-Private-Way-to-Stake/)
explains the motivation for creating vspd to replace dcrstakepool.

## Dependencies

vspd 1.0.0 requires:

- dcrd 1.6.0
- dcrwallet 1.6.0

When deploying vspd to production, always use release versions of all binaries.
Neither vspd nor its dependencies should be built from master when handling
mainnet tickets.

## New features

The key features offered by this initial version are:

- HTTP API
  - Endpoints to allows VSP users to register their tickets with the VSP, and to
    check on the status of registered tickets.
  - A status endpoint allows VSP operators to remotely monitor vspd.

- Web front-end
  - A public home page displays various VSP statistics.
  - A hidden admin page allows VSP operators to search for registered tickets
    and download database backups.

Please review the [project README](https://github.com/decred/vspd) for extended
documentation, including how to use the API and a detailed deployment guide.
