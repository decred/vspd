# API

## General notes

- Success responses use HTTP status 200 and a JSON encoded body.

- Error responses use either HTTP status 500 or 400, and a JSON encoded error
  in the body. For example `{"error":"Description"}`.

- Requests which reference specific tickets need to be properly signed as
  described in [two-way-accountability.md](./two-way-accountability.md).

- Implementation of request and response types can be found in
  [webapi/types.go](./webapi/types.go).

## Expected usage

### Get VSP public key

Clients should first retrieve the VSP's public key so they can check the
signature on later API responses. A VSP should never change their public key so
it can be requested once and cached indefinitely.

- `GET /api/pubkey`

    No request body.

    Response:
  
    ```json
    {
        "timestamp":1590509065,
        "pubkey":"bLNwVVcda3LqRLv+m0J5sjd60+twjO/fuhcx8RUErDQ="
    }
    ```

### Register ticket

**Registering a ticket is a two step process. The VSP will not add a ticket to
its voting wallets unless both of these calls have succeeded.**

Request fee amount and address for a ticket. The fee amount is only valid until
the expiration time has passed.

- `POST /feeaddress`

    Request:

    ```json
    {
        "timestamp":1590509066,
        "tickethash":"484a68f7148e55d05f0b64a29fe7b148572cb5272d1ce2438cf15466d347f4f4"
    }

    ```

    Response:

    ```json
    {
        "timestamp":1590509066,
        "feeaddress":"Tsfkn6k9AoYgVZRV6ZzcgmuVSgCdJQt9JY2",
        "fee":0.001,
        "expiration":1590563759,
        "request": {"<Copy of request body>"}
    }
    ```

Provide the voting key for the ticket, voting preference, and a signed
transaction which pays the fee to the specified address. If the fee has expired,
this call will return an error and the client will need to request a new fee by
calling `/feeaddress` again. The VSP will not broadcast the fee transaction
until the ticket purchase has 6 confirmations, and it will not add the ticket to
its voting wallets until the fee transaction has 6 confirmations.

- `POST /payfee`

    Request:

    ```json
    {
    "timestamp":1590509066,
    "tickethash":"484a68f7148e55d05f0b64a29fe7b148572cb5272d1ce2438cf15466d347f4f4",
    "feetx":"010000000125...737b266ffb9a93",
    "votingkey":"PtWUJWhSXsM9ztPkdtH8REe91z7uoidX8dsMChJUZ2spagm7YvrNm",
    "votechoices":{"headercommitments":"yes"}
    }
    ```

    Response:

    ```json
    {
    "timestamp":1590509066,
    "request": {"<Copy of request body>"}
    }
    ```

### Information requests

Clients can check the status of the server at any time.

// TODO

Clients can check the status of a ticket at any time after calling
`/feeaddress`.

- `GET /ticketstatus`

    Request:

    ```json
    {
        "timestamp":1590509066,
        "tickethash":"484a68f7148e55d05f0b64a29fe7b148572cb5272d1ce2438cf15466d347f4f4"
    }
    ```

    Response:

    ```json
    {
        "timestamp":1590509066,
        "status":"active",
        "votechoices":{"headercommitments":"no"},
        "request": {"<Copy of request body>"}
    }
    ```

### Update vote choices

Clients can update the voting preferences of their ticket at any time after
after calling `/payfee`.

- `POST /setvotechoices`

    Request:

    ```json
    {
      "timestamp":1590509066,
      "tickethash":"484a68f7148e55d05f0b64a29fe7b148572cb5272d1ce2438cf15466d347f4f4",
      "votechoices":{"headercommitments":"no"}
    }
    ```

    Response:

    ```json
    {
      "timestamp":1590509066,
      "votechoices":{"headercommitments":"no"},
      "request": {"<Copy of request body>"}
    }
    ```
