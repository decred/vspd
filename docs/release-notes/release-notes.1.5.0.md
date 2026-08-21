# vspd 1.5.0

vspd v1.5.0 contains all development work completed since v1.4.0 (November 2025).
All commits included in this release can be viewed
[on GitHub](https://github.com/decred/vspd/compare/release-v1.4.0...release-v1.5.0).

## Dependencies

vspd 1.5.0 must be built with go 1.26 or later, and requires:

- dcrd 2.1.6
- dcrwallet 2.1.6

Always use release versions of all binaries when deploying vspd to production.
Neither vspd nor its dependencies should be built from master when handling
mainnet tickets.

## Recommended Upgrade Procedure

The upgrade procedure below includes vspd downtime, during which clients will
not be able to register new tickets, check their ticket status, or update their
voting preferences. You may wish to put up a temporary maintenance webpage or
announce downtime in public channels. Voting on tickets already registered with
the VSP will not be interrupted.

1. Build vspd from the `release-v1.5.0` tag, and build dcrwallet and dcrd from
   their `release-v2.1.6` tags.
1. Stop vspd.
1. **Make a backup of the vspd database file in case rollback is required.**
1. Stop the instance of dcrd running on the vspd server.
1. Install new dcrd binary on the vspd server and start it to begin any required
   database upgrades. You can proceed with the following steps while the
   upgrades run.
1. Upgrade voting wallets one by one so at least two wallets remain online for
   voting at all times. On each server:
    1. Stop dcrwallet.
    1. Stop dcrd.
    1. Install new dcrd binary and start.
    1. Wait for any dcrd database upgrades to complete.
    1. Check dcrd log file for warnings or errors.
    1. Install new dcrwallet binary and start.
    1. Wait for any dcrwallet database upgrades to complete.
    1. Check dcrwallet log file for warnings or errors.
1. Ensure dcrd on the vspd server has completed all database upgrades.
1. Check dcrd log file for warnings or errors.
1. Install new vspd binary and start it.
1. Check vspd log file for warnings or errors.
1. Log in to the admin webpage and check the VSP Status tab for any issues.

## Notable Changes

- VSP revenue is now displayed on the admin page.

  Revenue figures are displayed in DCR and provided for the lifetime of the VSP,
  the previous 28 days, and the previous 24 hours.
  
- More strict dcrd/dcrwallet version checks.

  Previously only the RPC version of dcrd and dcrwallet was checked to ensure
  API compatability, however this was not enough to ensure the VSP was running
  the latest binary versions. Now the binary version is checked too, helping
  to ensure the VSP is using the latest voting agendas and running the
  latest security updates.

- SPV wallets explicitly prohibited.

  SPV mode voting wallets have never been supported by vspd, however this was
  not documented and not enforced. vspd will no longer connect to a wallet if it
  detects it is running in SPV mode and an error will be logged.

### Config Changes

No config changes since version 1.4.0.

### API changes

No API changes since version 1.4.0.

### Security

- Admin passwords are now checked using constant time comparison to reduce the
  risk of brute-forcing the password via a side-channel/timing attack.

- HTTP requests readers are now limited to 1 MiB to reduce the likelihood of
  memory exhaustion attacks.

- All HTTP endpoints now implement CSRF protection.

- A limit of 16 concurrent transaction broadcasts mitigates a potential DoS
  vector.

- Status JSON endpoint is now subject to the same rate limit as the login page
  to prevent password brute-forcing.

### Bug Fixes

- Fix handling of invalid tspend/treasury vote options
  ([#511](https://github.com/decred/vspd/pull/511)).

- Give webserver time for graceful shutdown
  ([#514](https://github.com/decred/vspd/pull/514)).

- Minor logging fixes
  ([#515](https://github.com/decred/vspd/pull/515),
  [#525](https://github.com/decred/vspd/pull/525)).

- Fix orphan transaction detection
  ([#517](https://github.com/decred/vspd/pull/517)).

- Delete vote changes for tickets which never got mined
  ([#526](https://github.com/decred/vspd/pull/526)).
