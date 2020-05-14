# dcrvsp

## Design decisions

- [gin-gonic](https://github.com/gin-gonic/gin) webserver
- [bbolt](https://github.com/etcd-io/bbolt) database
  - Tickets are stored in a single bucket, using ticket hash as the key and a
    json encoded representation of the ticket as the value.

## MVP features

- VSP API "v3" as described in [dcrstakepool #574](https://github.com/decred/dcrstakepool/issues/574)
and implemented in [dcrstakepool #625](https://github.com/decred/dcrstakepool/pull/625)
  - Request fee amount
  - Request fee address
  - Pay fee
  - Set voting preferences
- A minimal, static, web front-end providing pool stats and basic connection instructions.

## Future features

- Write database backups to disk periodically.
- Backup over http.
- Status check API call as described in [dcrstakepool #628](https://github.com/decred/dcrstakepool/issues/628).
- Accountability for both client and server changes to voting preferences.
- Consistency checking across connected wallets.
- Validate votebits provided in PayFee request are valid per current agendas.
