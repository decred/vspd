# vspd

[![Build Status](https://github.com/decred/vspd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/vspd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Go Report Card](https://goreportcard.com/badge/github.com/decred/vspd)](https://goreportcard.com/report/github.com/decred/vspd)
[![Release](https://img.shields.io/github/release/decred/vspd.svg?style=flat-square)](https://github.com/decred/vspd/releases/latest)

## Overview

<img src="./docs/img/stakey.png" align="right" />

vspd is a from scratch implementation of a Voting Service Provider (VSP) for
the Decred network.

A VSP running vspd can be used to vote on any ticket - tickets do not need to
be purchased with any special conditions such as dedicated outputs for paying
VSP fees. Fees are paid directly to the VSP with an independent on-chain
transaction.

To use vspd, ticket holders must prove ownership of their ticket with a
cryptographic signature, pay the fee requested by the VSP, and submit a private
key which enables the VSP to vote the ticket. Once this process is complete the
VSP will add the ticket to a pool of always-online voting wallets.

## Features

- **API** - Tickets are registered with the VSP using a JSON HTTP API. For more
  detail on the API and its usage, read [api.md](./docs/api.md)

- **Web front-end** - A minimal website (no JavaScript) providing public pool
  stats. An admin page enables searching for tickets and downloading database
  backups.

- **Two-way accountability** - All vspd requests and responses are signed by
  their sender, which enables both the client and the server to hold each other
  accountable in the case of misbehaviour. For more detail, and examples, read
  [two-way-accountability.md](./docs/two-way-accountability.md).

- **Dynamic fees** - Clients must request a new fee address and amount for every
  ticket. When these are given to a client, there is an associated expiry
  period. If the fee is not paid in this period, the client must request a new
  fee. This enables the VSP admin to change their fee as often as they like.

## Implementation

vspd is built and tested on go 1.16 and 1.17, making use of the following
libraries:

- [gin-gonic/gin](https://github.com/gin-gonic/gin) webserver.

- [etcd-io/bbolt](https://github.com/etcd-io/bbolt) k/v database.

- [jrick/wsrpc](https://github.com/jrick/wsrpc) for RPC communication with dcrd
  and dcrwallet.

## Deployment

A vspd deployment consists of a single front-end server which handles web
requests, and a number of remote servers which host voting wallets. For more
information about deploying vspd, check out
[deployment.md](./docs/deployment.md).

## Development

### Test Harness

A test harness is provided in `harness.sh`. The test harness uses tmux to start
a testnet instance of dcrd, multiple dcrwallets, and finally vspd. Further
documentation can be found in [harness.sh](./harness.sh).

### Web server debug mode

The config option `--webserverdebug` will:

- Force HTML templates to be reloaded on every web request.
- Reload the cached homepage data every second rather than every 5 minutes.
- Enable detailed webserver logging to the terminal (does not get written to log
  file).

## Issue Tracker

The [integrated GitHub issue tracker](https://github.com/decred/vspd/issues)
is used for this project.

## License

vspd is licensed under the [copyfree](http://copyfree.org) ISC License.
