# dcrvsp

[![Build Status](https://github.com/jholdstock/dcrvsp/workflows/Build%20and%20Test/badge.svg)](https://github.com/jholdstock/dcrvsp/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Go Report Card](https://goreportcard.com/badge/github.com/jholdstock/dcrvsp)](https://goreportcard.com/report/github.com/jholdstock/dcrvsp)

## Overview

User purchases a ticket, doesnt need any special conditions, indistinguishable
from solo ticket. User can then choose to use a VSP on a per-ticket basis. Once
the ticket is mined, and ideally before it has matured, the user sends the
ticket details + fee to a VSP, and the VSP will take the fee and vote in return.

## Advantages

### For Administrators

- bbolt db - no database admin required.
- Database is not used outside of dcrvsp server.
- No stakepoold.
- Client accountability.
- No need to use the same wallet seed on each voting wallet.
- Fees can change regularly - previously cached by wallet.

### For Users

- No redeem script to back up.
- No registration required. No email.
- Multiple VSPs on a single ticket.
- Voting preferences per ticket.
- Server accountability.
- No address reuse.
- VSP fees are paid "out of band", rather than being included in the ticket
  itself. This makes solo tickets and VSP tickets indistinguishable from
  eachother, enabling VSP users to purchase tickets in the same anonymity set
  as solo stakers.

## Design Decisions

- [gin-gonic](https://github.com/gin-gonic/gin) webserver.
  - Success responses use HTTP status 200 and a JSON encoded body.
  - Error responses use either HTTP status 500 or 400, and a JSON encoded error
    in the body (eg. `{"error":"Description"}')
- [bbolt](https://github.com/etcd-io/bbolt) k/v database.
  - Tickets are stored in a single bucket, using ticket hash as the key and a
    json encoded representation of the ticket as the value.
- [wsrpc](https://github.com/jrick/wsrpc) for RPC communication between dcrvsp
  and dcrwallet.

## Architecture

- Single server running dcrvsp and dcrd. dcrd requires txindex so
  `getrawtransaction` can be used.
- Multiple remote voting servers, each running dcrwallet and dcrd. dcrwallet
  on these servers should be constantly unlocked and have voting enabled.

## MVP Features

- When dcrvsp is started for the first time, it generates a ed25519 keypair and
  stores it in the database. This key is used to sign all API responses, and the
  signature is included in the response header `VSP-Server-Signature`. Error responses
  are not signed.
- Every client request which references a ticket should include a HTTP header
  `VSP-Client-Signature`. The value of this header must be a signature of the
  request body, signed with the commitment address of the referenced ticket.
- An xpub key is provided to dcrvsp via config. dcrvsp will use this key to
  derive addresses for fee payments. A new address is generated for each fee.
- VSP API as described in [dcrstakepool #574](https://github.com/decred/dcrstakepool/issues/574)
  - Request fee amount (`GET /fee`)
  - Request fee address (`POST /feeaddress`)
  - Pay fee (`POST /payFee`)
  - Ticket status (`GET /ticketstatus`)
  - Set voting preferences (`POST /setvotechoices`)
- A minimal, static, web front-end providing pool stats and basic connection
  instructions.
- Fees have an expiry period. If the fee is not paid within this period, the
  client must request a new fee. This enables the VSP to alter its fee rate.

## Future Features

- Write database backups to disk periodically.
- Backup over http.
- Status check API call as described in [dcrstakepool #628](https://github.com/decred/dcrstakepool/issues/628).
- Consistency checking across connected wallets.

## Backup

- Regular backups of bbolt database and feexpub.

## Issue Tracker

The [integrated github issue tracker](https://github.com/jholdstock/dcrvsp/issues)
is used for this project.

## License

dcrvsp is licensed under the [copyfree](http://copyfree.org) ISC License.
