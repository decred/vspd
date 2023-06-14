# vspd 1.2.0

vspd v1.2.0 contains all development work completed since v1.1.0 (March 2022).
Since then, 80 commits have been produced and merged by 3 contributors.
All commits included in this release can be viewed on GitHub
[here](https://github.com/decred/vspd/compare/release-v1.1.0...release-v1.2.0).

## Dependencies

vspd 1.2.0 must be built with go 1.19 or later, and requires:

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

1. Build vspd from the 1.2.0 release tag. Build dcrwallet and dcrd from their
   1.8.0 release tags.
1. Stop vspd.
1. Stop the instance of dcrd running on the vspd server.
1. **Make a backup of the vspd database file in case rollback is required.**
1. Install new dcrd binary on the vspd server and start it to begin database
   upgrades. You can proceed with the following steps while the upgrades run.
1. Upgrade voting wallets one at a time so at least two wallets remain online
   for voting. On each server:
    1. Stop dcrwallet.
    1. Stop dcrd.
    1. Install new dcrd binary and start.
    1. Wait for dcrd database upgrades to complete.
    1. Check dcrd log file for warnings or errors.
    1. Install new dcrwallet binary and start.
    1. Wait for dcrwallet database upgrades to complete.
    1. Check dcrwallet log file for warnings or errors.
1. Wait for dcrd on the vspd server to complete its database upgrade.
1. Check dcrd log file for warnings or errors.
1. Install new vspd binary and start it.
1. Check vspd log file for warnings or errors.
1. Log in to the admin webpage and check the VSP Status tab for any issues.

## Notable Changes

- vspd has moved into the cmd directory of the repo. This is standard practise
  in Decred repos which include more than one golang executable. The new command
  to build is:

  ```no-highlight
  go build ./cmd/vspd
  ```

- `/vspinfo` now returns `votingwalletsonline` and `votingwalletstotal`.
- The login form for the admin page is now limited to 3 attempts per second per IP.
- A warning is logged when running a pre-release version of vspd on mainnet.
- The ID of the git commit vspd was built from is now included in startup
  logging and in the output of `--version`.
- A new executable `vote-validator` is a tool for VSP admins to verify that
  their vspd deployment is voting correctly according to user preferences.
  Further details can be found in the [README](./cmd/vote-validator).
- A new executable `v3tool` is a development tool helpful for testing changes to
  vspd. Further details can be found in the [README](./cmd/v3tool).
