# dcrvsp

[![Build Status](https://github.com/jholdstock/dcrvsp/workflows/Build%20and%20Test/badge.svg)](https://github.com/jholdstock/dcrvsp/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Go Report Card](https://goreportcard.com/badge/github.com/jholdstock/dcrvsp)](https://goreportcard.com/report/github.com/jholdstock/dcrvsp)

## Design decisions

- [gin-gonic](https://github.com/gin-gonic/gin) webserver for both front-end and API.
  - API uses JSON encoded reqs/resps in HTTP body.
- [bbolt](https://github.com/etcd-io/bbolt) k/v database.
  - Tickets are stored in a single bucket, using ticket hash as the key and a
    json encoded representation of the ticket as the value.
- [wsrpc](https://github.com/jrick/wsrpc) for dcrwallet comms.

## MVP features

- VSP API as described in [dcrstakepool #574](https://github.com/decred/dcrstakepool/issues/574)
  - Request fee amount (`GET /fee`)
  - Request fee address (`POST /feeaddress`)
  - Pay fee (`POST /payFee`)
  - Ticket status (`POST /ticketstatus`)
  - Set voting preferences (`POST /setvotebits`)
- A minimal, static, web front-end providing pool stats and basic connection instructions.
- Fees have an expiry period. If the fee is not paid within this period, the
  client must request a new fee. This enables the VSP to alter its fee rate.

## Future features

- Write database backups to disk periodically.
- Backup over http.
- Status check API call as described in [dcrstakepool #628](https://github.com/decred/dcrstakepool/issues/628).
- Accountability for both client and server changes to voting preferences.
- Consistency checking across connected wallets.

## Notes

- dcrd must have transaction index enabled so `getrawtransaction` can be used.

## Issue Tracker

The [integrated github issue tracker](https://github.com/jholdstock/dcrvsp/issues)
is used for this project.

## License

dcrvsp is licensed under the [copyfree](http://copyfree.org) ISC License.