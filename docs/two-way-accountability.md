# Two-way Accountability

- When vspd is started for the first time, it generates a ed25519 keypair and
  stores it in the database. This key is used to sign all API responses, and the
  signature is included in the response header `VSP-Server-Signature`.

- Every client request which references a ticket should include a HTTP header
  `VSP-Client-Signature`. The value of this header must be a signature of the
  request body, signed with the commitment address of the referenced ticket.

## Examples

### Server does not vote ticket

### Client denies changing their voting preferences
