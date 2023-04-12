module github.com/decred/vspd

go 1.19

require (
	decred.org/dcrwallet/v3 v3.0.0
	github.com/decred/dcrd/blockchain/stake/v5 v5.0.0
	github.com/decred/dcrd/blockchain/standalone/v2 v2.1.1
	github.com/decred/dcrd/chaincfg/chainhash v1.0.3
	github.com/decred/dcrd/chaincfg/v3 v3.1.1
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.1.0
	github.com/decred/dcrd/dcrutil/v4 v4.0.0
	github.com/decred/dcrd/hdkeychain/v3 v3.1.0
	github.com/decred/dcrd/rpc/jsonrpc/types/v4 v4.0.0
	github.com/decred/dcrd/txscript/v4 v4.0.0
	github.com/decred/dcrd/wire v1.5.0
	github.com/decred/slog v1.2.0
	github.com/decred/vspd/client/v2 v2.0.0
	github.com/decred/vspd/types/v2 v2.0.0
	github.com/dustin/go-humanize v1.0.1
	github.com/gin-gonic/gin v1.8.2
	github.com/gorilla/sessions v1.2.1
	github.com/jessevdk/go-flags v1.5.0
	github.com/jrick/bitset v1.0.0
	github.com/jrick/logrotate v1.0.0
	github.com/jrick/wsrpc/v2 v2.3.5
	go.etcd.io/bbolt v1.3.7
)

replace (
	decred.org/dcrwallet/v3 => decred.org/dcrwallet/v3 v3.0.0-20230406144806-dc82294b976a
	github.com/decred/dcrd/blockchain/stake/v5 => github.com/decred/dcrd/blockchain/stake/v5 v5.0.0-20221022042529-0a0cc3b3bf92
	github.com/decred/dcrd/blockchain/standalone/v2 => github.com/decred/dcrd/blockchain/standalone/v2 v2.1.1-0.20230411184711-ce46220cf772
	github.com/decred/dcrd/gcs/v4 => github.com/decred/dcrd/gcs/v4 v4.0.0-20221022042529-0a0cc3b3bf92
	github.com/decred/dcrd/rpc/jsonrpc/types/v4 => github.com/decred/dcrd/rpc/jsonrpc/types/v4 v4.0.0-20221022042529-0a0cc3b3bf92
)

require (
	github.com/agl/ed25519 v0.0.0-20170116200512-5312a6153412 // indirect
	github.com/dchest/siphash v1.2.3 // indirect
	github.com/decred/base58 v1.0.4 // indirect
	github.com/decred/dcrd/crypto/blake256 v1.0.0 // indirect
	github.com/decred/dcrd/crypto/ripemd160 v1.0.1 // indirect
	github.com/decred/dcrd/database/v3 v3.0.0 // indirect
	github.com/decred/dcrd/dcrec v1.0.0 // indirect
	github.com/decred/dcrd/dcrec/edwards/v2 v2.0.2 // indirect
	github.com/decred/dcrd/dcrjson/v4 v4.0.0 // indirect
	github.com/gin-contrib/sse v0.1.0 // indirect
	github.com/go-playground/locales v0.14.0 // indirect
	github.com/go-playground/universal-translator v0.18.0 // indirect
	github.com/go-playground/validator/v10 v10.11.1 // indirect
	github.com/goccy/go-json v0.10.0 // indirect
	github.com/gorilla/securecookie v1.1.1 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/leodido/go-urn v1.2.1 // indirect
	github.com/mattn/go-isatty v0.0.17 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pelletier/go-toml/v2 v2.0.6 // indirect
	github.com/ugorji/go/codec v1.2.9 // indirect
	golang.org/x/crypto v0.6.0 // indirect
	golang.org/x/net v0.6.0 // indirect
	golang.org/x/sys v0.5.0 // indirect
	golang.org/x/text v0.7.0 // indirect
	golang.org/x/time v0.3.0
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
