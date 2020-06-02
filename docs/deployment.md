# Deployment Guide

This guide is deliberately written at a high level and with minimal details
because it is assumed that VSP operators will already have a level of
familiarity with Decred software and a level of sysadmin experience.

## Prerequisites

### Build from source

Compiled binaries are not provided for vspd - VSP operators are expected to
build vspd from source.

### Fee wallet

A wallet should be created to collect VSP fees. Ideally this would be a cold
wallet which is not used for any other purpose, and it should be completely
separate from the vspd infrastructure. The dcrwallet `getmasterpubkey` RPC
should be used to export an extended public (xpub) key from one of the wallet
accounts. This xpub key will be provided to vspd via config, and vspd will use
it to derive a new addresses for receiving fee payments.

## Front-end Server

The front-end server is where vspd will be running. The port vspd is listening
on (default `3000`) should be available for clients to reach over the internet.
This port is used for both the API and serving the HTML front end.

dcrd needs to be running on this server with transaction index enabled
(`--txindex`). dcrd is used for fishing ticket details out of the chain, for
receiving `blockconnected` notifications, and for broadcasting and checking the
status of fee transactions.

## Voting Servers

A vspd deployment should have a minimum of three remote voting wallets. The
servers hosting these wallets should ideally be in geographically seperate
locations.

Each voting server should be running an instance of dcrd and dcrwallet. The
wallet on these servers should be completely empty and not used of any other
purpose. dcrwallet should be permenantly unlocked and have voting enabled
(`--enablevoting`). vspd on the front-end server must be able to reach each
instance of dcrwallet over RPC.

## Deploying alongside dcrstakepool

It is possible to run vspd on the same infrastructure as an existing
dcrstakepool deployment.

- On the voting servers...
  - The existing dcrd instance requires no changes.
  - Create a new instance of dcrwallet listening for RPC connections on a
    different port. Ensure wallet is unlocked and voting is enabled
    (dcrstakepool wallet should have voting disabled).

- On the front-end server...
  - Run an instance of dcrd with txindex enabled.
  - Run the vspd binary and ensure the listening port can be reached over the
    internet. Configure vspd to use the newly created dcrwallet instances.

## Monitoring

// TODO

## Backup

The bbolt database file used by vspd is stored in the process home directory, at
the path `{homedir}/data/{network}/vspd.db`. vspd keeps a file lock on this
file, so it cannot be opened by any other processes while vspd is running.

To facilitate back-ups, vspd will periodically write a copy of the bbolt
database to the path `{homedir}/data/{network}/vspd.db-backup`. A copy of the
database file will also be written to this path when vspd shuts down.

## Disaster Recovery

// TODO
