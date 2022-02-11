# vspd 1.1.0

This release of vspd introduces several new features, as well as some important
performance improvements and bug fixes. Some of the key highlights are:

- Enabling users to set voting preferences for Treasury spend votes.
- Supporting tickets purchased with Trezor hardware wallets.
- Improvements to the web interface such as exposing more stats on the homepage,
  and a tabbed navigation system for the admin screen.
- Dramatically improving performance of the vspd database when it contains a
  large number of tickets.
- Several changes to the HTTP API.
- Several new config file settings.

Details of all notable changes and new features are provided in a section below.

vspd v1.1.0 contains all development work completed since
[v1.0.0](https://github.com/decred/vspd/releases/tag/release-v1.0.0) (January 2021).
Since then, 82 commits have been produced and merged by 5 contributors. All
commits included in this release can be viewed on GitHub
[here](https://github.com/decred/vspd/compare/release-v1.0.0...release-v1.1.0).

## Downgrade Warning

This release contains a backwards incompatible database upgrade.
The new database format is not compatible with previous versions of the vspd
software, and there is no code to downgrade the database back to the previous
version.

Making a copy of the database backup before running the upgrade is suggested
in order to enable rolling back to a previous version of the software if required.

## Dependencies

vspd 1.1.0 must be built with go 1.16 or later, and requires:

- dcrd 1.7.0
- dcrwallet 1.7.0

When deploying vspd to production, always use release versions of all binaries.
Neither vspd nor its dependencies should be built from master when handling
mainnet tickets.

## Recommended Upgrade Path

The upgrade path below includes vspd downtime, during which clients will not be
able to register new tickets, check their ticket status, or update their voting
preferences. Voting on tickets already registered with the VSP will not be
interrupted. You may wish to put up a temporary maintenance webpage or announce
downtime in public channels.

1. Build vspd from the 1.1.0 release tag. Build dcrwallet and dcrd from their
   1.7.0 release tags.
1. Stop vspd.
1. Stop the instance of dcrd running on the vspd server.
1. **Make a backup of the vspd database file in case rollback is required.**
1. Install new dcrd binary on the vspd server, start it and it will begin
  database upgrades. You can proceed with the following steps while the
  upgrades run.
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
1. Make any required changes to `vspd.conf`. New config items are detailed below.
1. Wait for dcrd on the vspd server to complete its database upgrade.
1. Check dcrd log file for warnings or errors.
1. Install new vspd binary and start it.
1. Check vspd log file for warnings or errors.
1. Log in to the admin webpage and check the VSP Status tab for any issues.

## Notable Changes

### Treasury Spend Voting

This release adds the ability for users to set their voting preferences for
transactions spending from the Decred Treasury.

Users can either set their voting preference for a single TSpend hash (a.k.a.
TSpend policy), or they can chose to set a preference for all TSpends signed by
a specific Treasury key (a.k.a. Treasury policy).
This corresponds directly to the API exposed by dcrwallet.

Voting policies can be set when a ticket is initially registered with vspd using
the `/payfee` endpoint, and they may subsequently be updated with the
`/setvotechoices` endpoint.

TSpend and Treasury policies will be returned from the `/ticketstatus` endpoint,
as well as being shown in ticket lookup section of the admin screen.
Any requests which set or update TSpend or Treasury policies will be recorded in
the same manner that requests updating consensus vote choices are currently
recorded.

### Trezor Support

Requests sent to vspd must be signed in order to prove ticket ownership.
Previously this signing would only be allowed with the commitment address of a
ticket, but in order to enable support for tickets purchased with a Trezor
hardware wallet, clients may now add alternate signing addresses to their
tickets.

A single alternate signing address can be added to each ticket by using a new
API call `/setaltsignaddr`. The initial call to this endpoint must still be
signed with the tickets commitment address, but any subsequent calls may be
signed with either the commitment address or the address provided in the
`/setaltsignaddr` call.

All requests and responses from this endpoint are added to the database using
existing mechanisms, to ensure
[two-way accountability](https://github.com/decred/vspd/blob/master/docs/two-way-accountability.md)
is maintained.

If a ticket has an alternate signing address it will be shown in ticket lookup
section of the admin screen, and it will be returned by the `/ticketstatus` API
call.

### Admin screen improvements

- The features of the admin screen are now separated into a tabbed interface.
  The tabs are:
  - VSP Status
  - Ticket Search
  - Database
  - Logout
- Show status of the vspd server's local dcrd instance (previously only voting
  wallet status was shown).
- Wallet status table redesigned to match Decred look and feel.
- Database file size is now shown.
- Ticket information:
  - Table split into "Ticket", "Fee" and "Vote Choice" sections.
  - Raw JSON is now formatted for improved readability.
  - Ticket fees are displayed in DCR rather than atoms.
  - Block explorer links added for hashes, blocks heights and addresses.
  - Ticket purchase height is displayed.
  - Date/times are now properly formatted instead of displaying raw Unix timestamps.

### General UI improvements

- Homepage now displays a message when the VSP is closed. This is configurable,
  although a sensible default is also provided.
- Large numbers are now comma separated.
- Network proportion added to displayed stats.
- Display number of a revoked tickets as proportion of all tickets.
- Include timezone in timestamps.
- Add cache-busting to all web resources to ensure web clients will always
  download new versions when resources change.
- Cleared up terminology around "**Consensus** vote choices", so as to
  distinguish from newly added "**Treasury** vote choices".

### Database Performance

In order to preemptively deal with scaling issues seen on vspd databases
containing very large numbers of tickets, a number of changes have been made to
the database code and the database storage format.

- Each ticket is now stored in its own database bucket. Previously tickets were
  stored in a single bucket as JSON encoded strings.
- Previously when searching through all tickets in the database with a filter,
  every ticket would be fully deserialized. Now only tickets which match the
  filter are deserialized.
- The fee payment raw transaction hex is now only stored in the database for as
  long as it is required. Once the transaction is confirmed on-chain, the hex is
  deleted from the database.

As a result of these changes:

- Database file size is reduced by ~30%. A database with 200,000 tickets can now
  be expected to use roughly 900 MB of disk space, and 400,000 tickets around
  1,700 MB.
- Count, search and insert times now scale linearly with number of tickets
  stored (previously scaled exponentially).
- Given a database containing 200,000 tickets:
  - Count time reduced by ~90%
  - Insert/search time reduced by ~65%

### API changes

- New endpoint `/setaltsignaddr` allows clients to add an alternate signing
  address to their tickets.
- Optional parameters added to `/payfee` and `/setvotechoices` for setting
  TSpend and Treasury voting preferences.
- Network proportion, closed message and best block height added to `/vspinfo`.
- Alternate signing address and Treasury/TSpend policies are returned by
  `/ticketstatus` response.
- Status of the vspd server's local dcrd instance is now included in the
  `/admin/status` response.

### Config Changes

- Behaviour of the internal log rotator can now be customized using new
  `maxlogsize` and `logstokeep` options. These values were previously hard-coded
  which limited the amount of log history which would be retained.
- A new option `vspclosedmsg` can be used to display a short message on the
  webpage when `vspclosed` is set to true. The message will also be returned by
  the `/vspinfo` API endpoint.

### Bug Fixes

- Terminate process on `SIGHUP` signal, not just `SIGTERM` or `SIGINT`
  ([#293](https://github.com/decred/vspd/pull/293)).
- Log client IPs from `X-Forwarded-For` or `X-Real-Ip` HTTP headers
  ([#308](https://github.com/decred/vspd/pull/308)).
- Ensure purchase height is always set for every ticket
  ([#277](https://github.com/decred/vspd/pull/277)).
- Wait for any running notification handlers to finish before closing RPC/DB
  connections ([#271](https://github.com/decred/vspd/pull/271)).
- Removed two data races in the API data caching code
  ([#273](https://github.com/decred/vspd/pull/273)).
- Ensure panics generated by web requests are logged to file and not only
  stdout. This includes logging a stack trace, and the full body and headers of
  the HTTP request which caused the panic
  ([#255](https://github.com/decred/vspd/pull/255)).
- Return an error response to clients if generating a fee address fails
  ([#240](https://github.com/decred/vspd/pull/240)).
- Avoid a potential divide by zero
  ([#282](https://github.com/decred/vspd/pull/282)).
- Ensure tickets which are revoked outside of the vspd deployment (eg. by a
  users ticketbuyer wallet) are identified as revoked by vspd.
  ([#301](https://github.com/decred/vspd/pull/301)).
- Add new vote choices to old ones, rather than destructively overwriting the
  old choices ([#323](https://github.com/decred/vspd/pull/323)).
