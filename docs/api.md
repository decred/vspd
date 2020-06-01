# API

## General notes

- Success responses use HTTP status 200 and a JSON encoded body.

- Error responses use either HTTP status 500 or 400, and a JSON encoded error
  in the body, e.g. `{"error":"Description"}`.

- Requests which reference specific tickets need to be properly signed as
  described in [two-way-accountability.md](./two-way-accountability.md).

- Implementation of request and response types can be found in
  [webapi/types.go](../webapi/types.go).

## Expected usage

### Get VSP info

Clients should retrieve the VSP's public key so they can check the signature on
future API responses. A VSP should never change their public key, so it can be
requested once and cached indefinitely. `vspclosed` indicates that the VSP is
not currently accepting new tickets. Calling `/feeaddress` when a VSP is closed
will result in an error.

- `GET /api/vspinfo`

    No request body.

    Response:
  
    ```json
    {
        "timestamp":1590599436,
        "pubkey":"SjAmrAqH7LScCUwM1qo5O6Cu7aKhrM1ORszgZwD7HmU=",
        "feepercentage":0.05,
        "vspclosed":false,
        "network":"testnet3"
    }
    ```

### Register ticket

**Registering a ticket is a two step process. The VSP will not add a ticket to
its voting wallets unless both of these calls have succeeded.**

#### Step One

Request fee amount and address for a ticket. The fee amount is only valid until
the expiration time has passed. The fee amount is an absolute value measured in
DCR. Returns an error if the specified ticket is not currently immature or live.

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
        "feeamount":0.001,
        "expiration":1590563759,
        "request": {"<Copy of request body>"}
    }
    ```

#### Step Two

Provide the voting key for the ticket, voting preference, and a signed
transaction which pays the fee to the specified address. If the fee has expired,
this call will return an error and the client will need to request a new fee by
calling `/feeaddress` again. Returns an error if the specified ticket is not
currently immature or live.

The VSP will not broadcast the fee transaction until the ticket purchase has 6
confirmations. For this reason, it is important that the client ensures the
output being spent in the transaction is not spent elsewhere.

The VSP will not add the ticket to its voting wallets until the fee transaction
has 6 confirmations.

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

### Ticket Status

Clients can check the status of a ticket at any time after calling
`/feeaddress`. The lifecycle of the ticket is represented with a set of boolean
fields:

- `ticketconfirmed` is true when the ticket transaction has 6 confirmations.
- `feetxreceived` is true when the VSP has received a valid fee transaction.
- `feetxbroadcast` is true when the VSP has broadcast the fee transaction.
- `feeconfirmed` is true when the fee transaction has 6 confirmations.

The VSP will only add tickets to the voting wallets when all four of these
conditions are met. `feetxhash` will only be populated if `feetxbroadcast` is
true.

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
      "ticketconfirmed":true,
      "feetxreceived":true,
      "feetxbroadcast":true,
      "feeconfirmed":false,
      "feetxhash": "e1c02b04b5bbdae66cf8e3c88366c4918d458a2d27a26144df37f54a2bc956ac",
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
      "request": {"<Copy of request body>"}
    }
    ```
