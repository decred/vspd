# vspd 1.3.0

vspd v1.3.0 contains all development work completed since v1.2.0 (June 2023).
All commits included in this release can be viewed
[on GitHub](https://github.com/decred/vspd/compare/release-v1.2.0...release-v1.3.0).

## Dependencies

vspd 1.3.0 must be built with go 1.20 or later, and requires:

- dcrd 1.8.0
- dcrwallet 1.8.0

When deploying vspd to production, always use release versions of all binaries.
Neither vspd nor its dependencies should be built from master when handling
mainnet tickets.

## Recommended Upgrade Path

The upgrade path below includes vspd downtime, during which clients will not be
able to register new tickets, check their ticket status, or update their voting
preferences. Voting on tickets already registered with the VSP will not be
interrupted. You may wish to put up a temporary maintenance webpage or announce
downtime in public channels.

The upgrade path assumes dcrd and dcrwallet are already version 1.8.0.

1. Build vspd from the 1.3.0 release tag.
1. Stop vspd.
1. **Make a backup of the vspd database file in case rollback is required.**
1. Install new vspd binary and start it.
1. Check vspd log file for warnings or errors.
1. Log in to the admin webpage and check the VSP Status tab for any issues.

## Notable Changes

- Fee calculation now takes the new block reward subsidy split from the activation
  of [DCP-0012](https://github.com/decred/dcps/blob/master/dcp-0012/dcp-0012.mediawiki)
  into consideration. In practice, this means that VSPs will begin charging
  marginally higher fees.
- vspd will now distinguish between tickets which are "missed" and tickets which
  are "expired". Previously these tickets would be considered as a single set
  labelled "expired". This is acheived using dcrd and
  [Golomb-Coded Set filters](https://github.com/decred/dcrd/tree/master/gcs#gcs).
- vspd 1.3.0 will perform a **one-time update of every revoked ticket in the
  database** the first time it is started. This may take a while for VSPs which
  have been active for a long time or have a large number of revoked tickets.
- Expired and missed tickets added to `/vspinfo`. Revoked is now deprecated.
- Minor improvements to web UI:
  - Homepage now displays expired and missed tickets instead of revoked tickets.
  - Homepage only displays the current network if it is not running on mainnet.
  - Admin page now displays decoded transactions in a human readable format
    in addition to the raw bytes.
- Logging has been reviewed to reduce unnecessary verbosity and to make better
  use of log levels. Scripts or monitoring solutions which parse logs may need
  to be updated.

### Bug Fixes

- Disable spell checking on admin page input for ticket hash
  ([#397](https://github.com/decred/vspd/pull/397)).
- Duplicate transactions will no longer cause an error when calling
  sendrawtransaction
  ([#398](https://github.com/decred/vspd/pull/398)).
- Calculate missed/expired/revoked ticket ratios as percentage of all tickets,
- not just voted tickets
  ([#417](https://github.com/decred/vspd/pull/417)).
- Web server returns explicit errors intead of zero values when cache is not ready
  ([#440](https://github.com/decred/vspd/pull/440)).
