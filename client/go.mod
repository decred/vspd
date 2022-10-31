module github.com/decred/vspd/client

go 1.19

require (
	github.com/decred/dcrd/txscript/v4 v4.0.0
	github.com/decred/vspd/types v0.0.0-00010101000000-000000000000
)

require (
	github.com/agl/ed25519 v0.0.0-20170116200512-5312a6153412 // indirect
	github.com/dchest/siphash v1.2.2 // indirect
	github.com/decred/base58 v1.0.3 // indirect
	github.com/decred/dcrd/chaincfg/chainhash v1.0.3 // indirect
	github.com/decred/dcrd/crypto/blake256 v1.0.0 // indirect
	github.com/decred/dcrd/crypto/ripemd160 v1.0.1 // indirect
	github.com/decred/dcrd/dcrec v1.0.0 // indirect
	github.com/decred/dcrd/dcrec/edwards/v2 v2.0.2 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1 // indirect
	github.com/decred/dcrd/wire v1.5.0 // indirect
	github.com/decred/slog v1.2.0 // indirect
)

replace github.com/decred/vspd/types => ../types
