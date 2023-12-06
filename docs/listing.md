# Listing a VSP on decred.org

Public VSP servers are a key part of the Decred infrastructure as they make
Proof-of-Stake far more accessible for the average user.
It is therefore desirable to increase the number of public VSPs listed in
Decrediton and on [decred.org](https://decred.org/vsp) in order to promote
decentralization and improve the stability of the Decred network.

## Operator Requirements

* Familiarity with system administration work on Linux.
* Ability to compile from source, setting up and maintaining `dcrd` and
  `dcrwallet`.
* Willingness to stay in touch with the Decred community for important news and
  updates. A private channel on [Matrix](https://chat.decred.org) exists for
  this purpose.
* Availability to update VSP binaries when new releases are produced.
* Operators should ideally be pairs of individuals or larger groups, so that the
  unavailability of a single person does not lead to extended outages in their
  absence.
* Ability to effectively communicate in English.

## Deployment Requirements

* At least one machine dedicated to hosting the web front end, handling web
  connections from VSP users.
* At least three dedicated machines hosting voting wallets and a local instance
  of dcrd.
* The machines used to host the voting wallets must be spread across 3 or more
  physically separate locations.
* The web frontend must have an IP that is distinct from those of the voting
  wallets, and is ideally located in another physical location.
* The VSP must be run on testnet for 1 week to confirm it is working properly.
  Uptime and number of votes made versus missed will be checked.
* The VSP must be run on mainnet in test mode (no public access) until a VSP
  operator demonstrates they have successfully voted 1 ticket of their own using
  the VSP.
* The operator must have an adequate monitoring solution in place, ideally
  alerting on server downtime and application error logging.

## Further Information

For further support you can contact the [Decred community](https://decred.org/community).
