# vspadmin

vspadmin is a tool to perform various VSP administration tasks.

## Usage

```no-highlight
vspadmin [OPTIONS] COMMAND
```

## Options

```no-highlight
--homedir=                         Path to application home directory. (default: /home/user/.vspd)
--network=[mainnet|testnet|simnet] Decred network to use. (default: mainnet)
-h, --help                         Show help message
```

## Commands

### `createdatabase`

Creates a new database for a new deployment of vspd. Accepts the xpub key to be
used for collecting fees as a parameter.

Example:

```no-highlight
$ go run ./cmd/vspadmin createdatabase <xpub>
```

### `writeconfig`

Writes a config file with default values to the application home directory.

Example:

```no-highlight
$ go run ./cmd/vspadmin writeconfig
```

### `retirexpub`

Replaces the currently used xpub with a new one. Once an xpub key has been
retired it can not be used by the VSP again.

**Note:** vspd must be stopped before this command can be used because it
modifies values in the vspd database.

Example:

```no-highlight
$ go run ./cmd/vspadmin retirexpub <xpub>
```
