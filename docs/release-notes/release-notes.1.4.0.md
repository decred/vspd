# vspd 1.4.0

vspd v1.4.0 contains all development work completed since v1.3.2 (November 2023).
All commits included in this release can be viewed
[on GitHub](https://github.com/decred/vspd/compare/release-v1.3.2...release-v1.4.0).

## Downgrade Warning

This release contains a backwards incompatible database upgrade.
The new database format is not compatible with previous versions of the vspd
software, and there is no code to downgrade the database back to the previous
version.

Making a copy of the database backup before running the upgrade is suggested
in order to enable rolling back to a previous version of the software if required.

## Dependencies

vspd 1.4.0 must be built with go 1.23 or later, and requires:

- dcrd 2.0.6
- dcrwallet 2.0.6

Always use release versions of all binaries when deploying vspd to production.
Neither vspd nor its dependencies should be built from master when handling
mainnet tickets.

## Recommended Upgrade Procedure

The upgrade procedure below includes vspd downtime, during which clients will
not be able to register new tickets, check their ticket status, or update their
voting preferences. You may wish to put up a temporary maintenance webpage or
announce downtime in public channels. Voting on tickets already registered with
the VSP will not be interrupted.

1. Build vspd from the `release-v1.4.0` tag, and build dcrwallet and dcrd from
   their `release-v2.0.6` tags.
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

- A new executable named vspadmin has been added to the repository.

  vspadmin is a tool to perform various VSP administration tasks such as
  initializing new databases and creating default config files for fresh vspd
  deployments. It also enables operators of existing VSPs to change the extended
  public keys (xpub) used for collecting fees, something which was previously
  not possible.

  Full documentation for vspadmin can be found
  [on GitHub](https://github.com/decred/vspd/blob/master/cmd/vspadmin/README.md).

- The current and any historic fee xpub keys are listed on a new tab in the admin
  page.

- Fee calculation now takes the new block reward subsidy split from the activation
  of [DCP-0012](https://github.com/decred/dcps/blob/master/dcp-0012/dcp-0012.mediawiki)
  into consideration. In practice, this means that VSPs will begin charging
  marginally higher fees.

### Config Changes

- The vspd flag `--feexpub` is now deprecated and does nothing. The equivalent
  functionality has been moved into the `createdatabase` command of the new
  vspadmin executable.

- The vspd flag `--configfile` is now deprecated and does nothing. It is still
  possible to run vspd with config in a non-default location using the
  `--homedir` flag.

### API changes

- After being deprecated in release 1.3.0, the revoked ticket count has now been
  removed from `/vspinfo`. The number of revoked tickets can be calculated
  by adding the number of missed and expired tickets.

### Bug Fixes

- Don't run upgrades unnecessarily on brand new databases
  ([#477](https://github.com/decred/vspd/pull/477)).
- Don't initialize databases with private keys, only public
  ([#478](https://github.com/decred/vspd/pull/478)).
- Various minor GUI improvements and bugfixes
  ([#495](https://github.com/decred/vspd/pull/495)).
