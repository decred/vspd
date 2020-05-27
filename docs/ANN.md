# Announcement

## Advantages vs dcrstakepool

### For VSP Administrators

- An instance of bbolt db on the front-end server is used as the single source
  of truth:
  - bbolt does not have the sys admin overhead associated with maintaining a
      MySQL database. The database will be automatically created and maintained
      by vspd.
  - The bbolt database is only accessed by vspd. There is no need to open
      additional ports on your front-end server for the voting wallets to access
      the database.
- Voting wallet servers require only dcrwallet and dcrd. There is no longer a
  VSP binary (ie. stakepoold) running on voting servers.
- Voting servers no longer need dcrd to be running with `--txindex`.
- No need to use the same wallet seed on each voting wallet.
- A new fee address and amount are requested for each ticket:
  - Fee addresses are never reused.
  - Fee amount can be changed freely.
- No emails or personal information are held. No need to worry about GDPR et al.

### For VSP Users

- No redeem script to back up.
- No registration required - no email, no password, no CAPTCHA.
- Voting preferences can be set for each individual ticket.
- No address reuse.
- VSP fees are paid independently of the ticket purchase, rather than being
  included in the ticket:
  - Multiple VSPs can be used for a single ticket.
  - Fees can be paid using funds from a mixed account.
  - VSP users can purchase tickets in the same anonymity set at solo stakers.

### For the Decred Ecosystem

- Solo tickets and VSP tickets are indistinguishable on-chain.
- Clients and servers can hold eachother accountable for actions. This enables
  users to prove if a VSP is misbehaving, and VSPs to defend themselves if they
  are falsely accused.

## Disadvantages

- Front-end is more important than before.
- Front-end requires dcrd with `--txindex`.
- Failure cases
  - fee tx doesnt broadcast
